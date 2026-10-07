package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Mvnshi/legatus/internal/agent/claude"
	"github.com/Mvnshi/legatus/internal/agent/codex"
	"github.com/Mvnshi/legatus/internal/pool"
)

const accountsUsage = `Usage:
  legatus accounts add --id NAME --provider codex|claude [--home DIR|default] [--max N] [--label TEXT]
  legatus accounts ls
  legatus accounts login NAME     sign in to that login (you do the sign-in; Legatus never sees your password)
  legatus accounts enable|disable NAME
  legatus accounts rm NAME

An account is one login of one agent, kept in its own folder so several can be used side by side.
--home default uses the login the agent already has on this computer.
Use only logins you are entitled to use, within each provider's terms.
`

var accountID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func cmdAccounts(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, accountsUsage)
		return 64
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return accountsAdd(rest, stdout, stderr)
	case "ls", "list":
		return accountsList(rest, stdout, stderr)
	case "login":
		return accountsLogin(rest, stdout, stderr)
	case "enable", "disable":
		return accountsToggle(sub == "disable", rest, stdout, stderr)
	case "rm", "remove":
		return accountsRemove(rest, stdout, stderr)
	}
	fmt.Fprintf(stderr, "legatus: unknown accounts command %q\n\n%s", sub, accountsUsage)
	return 64
}

func accountsAdd(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("accounts add", stderr)
	id := fs.String("id", "", "a short name for this login, e.g. work or second")
	provider := fs.String("provider", "", "codex or claude")
	home := fs.String("home", "", "the login's folder, or \"default\" for the agent's normal login (default: a new folder)")
	max := fs.Int("max", 1, "how many tasks may use this login at once")
	label := fs.String("label", "", "a description shown in listings")
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if !accountID.MatchString(*id) {
		fmt.Fprintln(stderr, "legatus: --id must be 1-32 lowercase letters, digits, - or _")
		return 64
	}
	if *provider != "codex" && *provider != "claude" {
		fmt.Fprintln(stderr, "legatus: --provider must be codex or claude")
		return 64
	}
	if *max < 1 {
		fmt.Fprintln(stderr, "legatus: --max must be at least 1")
		return 64
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	for _, s := range a.Pool.Snapshots() {
		if s.ID == *id {
			return fail(stderr, fmt.Errorf("an account named %q already exists", *id))
		}
	}
	dir := *home
	switch dir {
	case "":
		dir = a.AccountHome(*provider, *id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fail(stderr, err)
		}
	case "default":
		dir = ""
	default:
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fail(stderr, err)
		}
	}
	if err := a.Pool.Add(pool.Account{ID: *id, Provider: *provider, Label: *label, Home: dir, MaxConcurrent: *max}); err != nil {
		return fail(stderr, err)
	}
	if err := a.SaveAccounts(); err != nil {
		return fail(stderr, err)
	}
	if dir == "" {
		fmt.Fprintf(stdout, "Added %s (%s), using the login %s already has on this computer.\n", *id, *provider, *provider)
		return 0
	}
	fmt.Fprintf(stdout, "Added %s (%s) with its own folder %s\nSign in to it now:\n\n  legatus accounts login %s\n", *id, *provider, dir, *id)
	return 0
}

func accountsList(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("accounts ls", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	snaps := a.Pool.Snapshots()
	if len(snaps) == 0 {
		fmt.Fprintln(stdout, "No accounts yet. Add one with: legatus accounts add --id main --provider codex --home default")
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tPROVIDER\tSTATE\tIN USE\tFOLDER")
	for _, s := range snaps {
		state := "ready"
		switch {
		case s.Disabled:
			state = "disabled"
		case s.LimitedUntil.After(time.Now()):
			state = "at its limit until " + s.LimitedUntil.Local().Format("Mon 15:04")
		}
		folder := s.Home
		if folder == "" {
			folder = "(the agent's normal login)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d/%d\t%s\n", s.ID, s.Provider, state, s.Active, max1(s.MaxConcurrent), folder)
	}
	tw.Flush()
	return 0
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

func accountsLogin(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("accounts login", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "Usage: legatus accounts login <name>")
		return 64
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	var acct *pool.Snapshot
	for _, s := range a.Pool.Snapshots() {
		if s.ID == fs.Arg(0) {
			s := s
			acct = &s
		}
	}
	if acct == nil {
		return fail(stderr, fmt.Errorf("no account named %q", fs.Arg(0)))
	}
	be, ok := a.Engine.Backends[acct.Provider]
	if !ok {
		return fail(stderr, fmt.Errorf("no backend for %s", acct.Provider))
	}
	var cmd *exec.Cmd
	switch acct.Provider {
	case "codex":
		cmd = exec.Command(codex.Executable(), "login")
	case "claude":
		fmt.Fprintln(stdout, "Claude Code will open. Type /login, finish signing in, then /exit.")
		cmd = exec.Command(claude.Executable())
	default:
		return fail(stderr, fmt.Errorf("don't know how to sign in to %s", acct.Provider))
	}
	cmd.Env = os.Environ()
	for k, v := range be.AccountEnv(acct.Account) {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return fail(stderr, fmt.Errorf("%s is not installed or not on PATH", cmd.Args[0]))
		}
		return fail(stderr, err)
	}
	return 0
}

func accountsToggle(disable bool, args []string, stdout, stderr io.Writer) int {
	fs := newFlags("accounts", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "Usage: legatus accounts enable|disable <name>")
		return 64
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	if !a.Pool.SetDisabled(fs.Arg(0), disable) {
		return fail(stderr, fmt.Errorf("no account named %q", fs.Arg(0)))
	}
	if err := a.SaveAccounts(); err != nil {
		return fail(stderr, err)
	}
	word := "enabled"
	if disable {
		word = "disabled"
	}
	fmt.Fprintf(stdout, "%s %s\n", fs.Arg(0), word)
	return 0
}

func accountsRemove(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("accounts rm", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "Usage: legatus accounts rm <name>")
		return 64
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	found := false
	for _, s := range a.Pool.Snapshots() {
		found = found || s.ID == fs.Arg(0)
	}
	if !found {
		return fail(stderr, fmt.Errorf("no account named %q", fs.Arg(0)))
	}
	a.Pool.Remove(fs.Arg(0))
	if err := a.SaveAccounts(); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "Removed %s. Its folder was left alone; delete it yourself if you want the login gone.\n", fs.Arg(0))
	return 0
}
