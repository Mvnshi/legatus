// Package cli is the `legatus` command line.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Mvnshi/legatus/internal/app"
)

// Version is set at build time.
var Version = "dev"

const usage = `Legatus runs coding agents in parallel on your own machine and never stops at a usage limit.

Usage:
  legatus run [flags] "what to do"     run one task through a workflow, in this repository
  legatus serve                        run the daemon: a queue, many runs at once, and the web cockpit
  legatus open                         open the cockpit in your browser
  legatus queue "what to do"           add a task to the running daemon
  legatus pr <run>                     push a finished run's branch and open a pull request
  legatus resume <run>                 continue a run that was interrupted
  legatus runs [--status <status>]     list runs, optionally filtered by status
  legatus show <run>                   show a run and its evidence report
  legatus accounts <add|ls|login|rm|enable|disable>
                                       manage the logins work is spread across
  legatus automations [check|example]  schedules and GitHub label watchers the daemon runs for you
  legatus clean                        remove the worktrees of finished runs
  legatus doctor                       check the tools and logins Legatus needs
  legatus demo                         watch a usage limit being handled, with no agent installed
  legatus version

Run "legatus <command> -h" for a command's flags.
`

// Main runs the command line and returns the process exit code.
//
// Exit codes: 0 success, 1 the run failed or a command failed, 2 the run needs a person's decision,
// 64 the command line was wrong.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version", "--version":
		fmt.Fprintln(stdout, "legatus", currentVersion())
		return 0
	case "run":
		return cmdRun(rest, stdout, stderr)
	case "resume":
		return cmdResume(rest, stdout, stderr)
	case "runs":
		return cmdRuns(rest, stdout, stderr)
	case "show":
		return cmdShow(rest, stdout, stderr)
	case "accounts":
		return cmdAccounts(rest, stdout, stderr)
	case "doctor":
		return cmdDoctor(rest, stdout, stderr)
	case "demo":
		return cmdDemo(rest, stdout, stderr)
	case "clean":
		return cmdClean(rest, stdout, stderr)
	case "pr":
		return cmdPR(rest, stdout, stderr)
	case "automations":
		return cmdAutomations(rest, stdout, stderr)
	case "serve":
		return cmdServe(rest, stdout, stderr)
	case "open":
		return cmdOpen(rest, stdout, stderr)
	case "queue":
		return cmdQueue(rest, stdout, stderr)
	}
	fmt.Fprintf(stderr, "legatus: unknown command %q\n\n%s", cmd, usage)
	return 64
}

// newFlags makes a flag set that reports to stderr and never exits the process.
func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("legatus "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// parse reports a flag error as exit code 64, and -h as success.
func parse(fs *flag.FlagSet, args []string) (code int, ok bool) {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0, false
		}
		return 64, false
	}
	return 0, true
}

// stringList is a flag that can be repeated.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ", ") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

func openApp(rootFlag string, stderr io.Writer) (*app.App, bool) {
	root := rootFlag
	if root == "" {
		var err error
		if root, err = app.Root(); err != nil {
			fmt.Fprintln(stderr, "legatus:", err)
			return nil, false
		}
	}
	a, err := app.Open(root, app.Options{})
	if err != nil {
		fmt.Fprintln(stderr, "legatus:", err)
		return nil, false
	}
	return a, true
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "legatus:", err)
	return 1
}
