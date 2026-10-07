package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/agent/claude"
	"github.com/Mvnshi/legatus/internal/agent/codex"
)

func cmdDoctor(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("doctor", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	problems := 0
	line := func(mark, format string, a ...any) { fmt.Fprintf(stdout, " %s  %s\n", mark, fmt.Sprintf(format, a...)) }
	version := func(exe string, args ...string) (string, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), true
	}

	fmt.Fprintln(stdout, "Tools")
	if v, ok := version("git", "--version"); ok {
		line("ok", "%s", v)
	} else {
		line("!!", "git is not installed or not on PATH. Legatus needs it (every task works in its own git worktree).")
		problems++
	}
	if v, ok := version(codex.Executable(), "--version"); ok {
		line("ok", "%s", v)
	} else {
		line("--", "codex is not on PATH (needed for codex accounts). Install the Codex CLI, or point Legatus at it.")
	}
	if v, ok := version(claude.Executable(), "--version"); ok {
		line("ok", "claude %s", v)
		line("--", "the claude backend is untested against a real install; treat its first runs as a trial")
	} else {
		line("--", "claude is not on PATH (needed for claude accounts)")
	}

	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "\nFiles\n")
	line("ok", "Legatus keeps its files in %s", a.Root)

	fmt.Fprintf(stdout, "\nAccounts\n")
	snaps := a.Pool.Snapshots()
	if len(snaps) == 0 {
		line("!!", "none yet: legatus accounts add --id main --provider codex --home default")
		problems++
	}
	home, _ := os.UserHomeDir()
	for _, s := range snaps {
		switch s.Provider {
		case "codex":
			dir := s.Home
			if dir == "" {
				dir = filepath.Join(home, ".codex")
			}
			if _, err := os.Stat(filepath.Join(dir, "auth.json")); err != nil {
				line("!!", "%s: not signed in yet (no login found in %s). Run: legatus accounts login %s", s.ID, dir, s.ID)
				problems++
				continue
			}
			line("ok", "%s: codex login found", s.ID)
		case "claude":
			line("--", "%s: claude login not checked (it can live in the system keychain). Run a small task to confirm.", s.ID)
		}
		if s.LimitedUntil.After(time.Now()) {
			line("--", "%s is at its usage limit until %s", s.ID, s.LimitedUntil.Local().Format("Mon 15:04"))
		}
	}
	fmt.Fprintln(stdout)
	if problems > 0 {
		fmt.Fprintf(stdout, "%d thing(s) to fix before running tasks.\n", problems)
		return 1
	}
	fmt.Fprintln(stdout, "Ready.")
	return 0
}
