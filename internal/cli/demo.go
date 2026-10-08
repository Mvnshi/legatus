package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Mvnshi/legatus/internal/engine"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/workflow"
)

// cmdDemo runs a scripted scenario with stand-in agents: the first login runs out of usage halfway
// through, the work moves to the second, the checks still run, and a different agent reviews it.
func cmdDemo(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("demo", stderr)
	keep := fs.Bool("keep", false, "keep the demo's files instead of deleting them")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	tmp, err := os.MkdirTemp("", "legatus-demo-")
	if err != nil {
		return fail(stderr, err)
	}
	if !*keep {
		defer os.RemoveAll(tmp)
	}

	repo := filepath.Join(tmp, "repo")
	if err := makeDemoRepo(repo); err != nil {
		return fail(stderr, err)
	}

	a, err := demoApp(tmp, 0)
	if err != nil {
		return fail(stderr, err)
	}
	a.Engine.OnEvent = func(ev model.Event) { printEvent(stdout, ev) }

	fmt.Fprintln(stdout, "Legatus demo: stand-in agents, nothing real is called.")
	fmt.Fprintln(stdout, "Two codex logins and one claude login. The first codex login will run out of usage halfway.")
	fmt.Fprintln(stdout)

	wf := workflow.Default([]string{"git --version"}, true)
	ctx := context.Background()
	run, err := a.Engine.NewRun(ctx, model.Task{Prompt: "Add the new feature", Repo: repo, Base: "main"}, wf)
	if err != nil {
		return fail(stderr, err)
	}
	if err := a.Engine.Execute(ctx, run.ID, wf); err != nil {
		return fail(stderr, err)
	}
	done, _ := a.Store.Load(run.ID)
	fmt.Fprintln(stdout)
	if done.Status != model.Succeeded {
		fmt.Fprintf(stdout, "The demo did not finish as expected: %s %s\n", done.Status, done.Error)
		return 1
	}
	out, _ := exec.Command("git", "-C", done.Worktree, "log", "--oneline", done.BaseCommit+"..HEAD").Output()
	fmt.Fprintf(stdout, "Result: the work finished on %s even though work-1 ran out of usage.\nCommits on the branch:\n%s", done.Branch, indent(string(out)))
	fmt.Fprintln(stdout, "\nThe report a person would read before merging:")
	evs, _, _ := a.Store.Events(done.ID, 0)
	stat, _ := exec.Command("git", "-C", done.Worktree, "diff", "--stat", done.BaseCommit, "HEAD").Output()
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, indent(engine.Evidence(done, evs, string(stat))))
	if *keep {
		fmt.Fprintf(stdout, "\nFiles kept in %s\n", tmp)
	}
	return 0
}

func makeDemoRepo(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	steps := [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Legatus Demo"},
		{"config", "user.email", "demo@localhost"},
	}
	for _, s := range steps {
		if out, err := exec.Command("git", append([]string{"-C", dir}, s...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v: %s", s, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0o600); err != nil {
		return err
	}
	for _, s := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "first commit"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, s...)...).CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v: %s", s, err, out)
		}
	}
	return nil
}

func indent(s string) string {
	out := ""
	for _, l := range splitLines(s) {
		out += "  " + l + "\n"
	}
	return out
}

func splitLines(s string) []string {
	var lines []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, cur)
			cur = ""
			continue
		}
		if r != '\r' {
			cur += string(r)
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
