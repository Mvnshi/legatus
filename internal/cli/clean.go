package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Mvnshi/legatus/internal/model"
)

// cmdClean removes the worktrees of finished runs. The run records and reports stay; the branch stays too
// unless asked, because it holds the work.
func cmdClean(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("clean", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	older := fs.Duration("older-than", 0, "only runs that finished at least this long ago, e.g. 72h")
	deleteBranch := fs.Bool("delete-branches", false, "also delete the branches (the work on them is lost unless merged)")
	failedOnly := fs.Bool("failed-only", false, "only runs that failed or were canceled")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	runs, err := a.Store.List()
	if err != nil {
		return fail(stderr, err)
	}
	removed := 0
	for _, r := range runs {
		if !r.Status.Terminal() {
			continue
		}
		if *failedOnly && r.Status == model.Succeeded {
			continue
		}
		if *older > 0 && time.Since(r.UpdatedAt) < *older {
			continue
		}
		if _, err := os.Stat(r.Worktree); err != nil {
			continue // already gone
		}
		if err := a.Engine.Worktrees.Remove(context.Background(), r.Task.Repo, r.Worktree, r.Branch, *deleteBranch); err != nil {
			fmt.Fprintf(stderr, "legatus: could not remove the worktree of %s: %v\n", r.ID, err)
			continue
		}
		fmt.Fprintf(stdout, "removed the worktree of %s (%s)\n", r.ID, r.Status)
		removed++
	}
	if removed == 0 {
		fmt.Fprintln(stdout, "Nothing to clean.")
		return 0
	}
	if *deleteBranch {
		fmt.Fprintf(stdout, "Removed %d worktree(s) and their branches.\n", removed)
	} else {
		fmt.Fprintf(stdout, "Removed %d worktree(s). The branches are still in your repositories: git branch --list \"legatus/*\"\n", removed)
	}
	return 0
}
