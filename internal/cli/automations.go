package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"

	"github.com/Mvnshi/legatus/internal/automation"
)

const exampleAutomations = `# Legatus automations. Save this as automations.yaml in your Legatus folder (legatus automations path
# prints where); the daemon (legatus serve) notices edits by itself. Check it with: legatus automations check

automations:
  # A task on a schedule. It first runs at the next scheduled time after you add it, never at once.
  # Schedules: "every 6h" (at least every 5m), "daily 09:00", "weekdays 09:00", "weekly mon 09:00".
  - id: weekly-deps
    schedule: weekly mon 09:00
    repo: C:/code/my-app          # the local checkout the work happens in (a full path)
    prompt: Update the dependencies, fix whatever breaks, and keep the tests passing.
    checks: ["npm test"]
    review: true                  # an independent agent reviews the result
    pr: draft                     # open a draft pull request when it succeeds (draft or ready)
    max_active: 1                 # do not start another while one is still going

  # Pick up GitHub issues. ONLY issues written by the logins listed under authors are taken, because
  # anyone who could write or label an issue could otherwise steer an agent on this computer.
  - id: labelled-issues
    github:
      repo: your-name/my-app
      label: legatus              # issues carrying this label
      every: 10m                  # how often to look (at least 1m)
      authors: [your-name]        # required; or anyone: true to accept the risk
      max_new: 2                  # at most this many new runs per look
    repo: C:/code/my-app
    checks: ["npm test"]
    pr: draft
    max_active: 2
`

func cmdAutomations(args []string, stdout, stderr io.Writer) int {
	sub := "ls"
	rest := args
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		sub, rest = args[0], args[1:]
	}
	fs := newFlags("automations "+sub, stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, rest); !ok {
		return code
	}
	if sub == "example" {
		fmt.Fprint(stdout, exampleAutomations)
		return 0
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	path := filepath.Join(a.Root, "automations.yaml")
	switch sub {
	case "path":
		fmt.Fprintln(stdout, path)
		return 0
	case "ls", "list", "check":
		cfg, err := automation.LoadConfig(path)
		if err != nil {
			fmt.Fprintf(stderr, "legatus: %v\n", err)
			return 1
		}
		if len(cfg.Automations) == 0 {
			fmt.Fprintf(stdout, "No automations. Write %s (see: legatus automations example).\n", path)
			return 0
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tKIND\tWHEN\tSTATE")
		for _, au := range cfg.Automations {
			kind, when := "schedule", au.Schedule
			if au.GitHub != nil {
				kind = "github"
				who := fmt.Sprintf("authors %v", au.GitHub.Authors)
				if au.GitHub.Anyone {
					who = "ANYONE"
				}
				when = fmt.Sprintf("label %q in %s every %s, %s", au.GitHub.Label, au.GitHub.Repo, au.GitHub.WatchEvery(), who)
			}
			state := "on"
			if au.Disabled {
				state = "off"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", au.ID, kind, when, state)
		}
		tw.Flush()
		fmt.Fprintf(stdout, "\n%s is valid.\n", path)
		return 0
	}
	fmt.Fprintf(stderr, "legatus: unknown automations command %q (ls, check, example, path)\n", sub)
	return 64
}
