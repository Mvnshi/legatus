package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/engine"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/workflow"
)

func cmdRun(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("run", stderr)
	repo := fs.String("repo", ".", "the git repository to work in")
	base := fs.String("base", "HEAD", "the branch or commit to start from")
	wfPath := fs.String("workflow", "", "a workflow file (default: one agent step, then your checks)")
	var checks stringList
	fs.Var(&checks, "check", "a command that must pass, e.g. \"go test ./...\" (repeatable)")
	review := fs.Bool("review", false, "have an independent agent review the change")
	agentFlag := fs.String("agent", "any", "which agent: any, codex, claude")
	pii := fs.Bool("redact-emails", false, "also remove email addresses from prompts")
	root := fs.String("root", "", "where Legatus keeps its files (default: LEGATUS_HOME or your config folder)")
	promptFile := fs.String("prompt-file", "", "read the task from a file (- for standard input)")
	quiet := fs.Bool("quiet", false, "do not print progress")
	noSandbox := fs.Bool("no-sandbox", false, "run agents without their own sandbox (they can reach what your user account can)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: legatus run [flags] \"what to do\"")
		fs.PrintDefaults()
	}
	if code, ok := parse(fs, args); !ok {
		return code
	}

	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if *promptFile != "" {
		var data []byte
		var err error
		if *promptFile == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(*promptFile)
		}
		if err != nil {
			return fail(stderr, err)
		}
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		fmt.Fprintln(stderr, "legatus: say what to do, e.g. legatus run \"fix the failing login test\"")
		return 64
	}

	var wf *workflow.Workflow
	if *wfPath != "" {
		var err error
		if wf, err = workflow.Load(*wfPath); err != nil {
			return fail(stderr, err)
		}
	} else {
		wf = workflow.Default(checks, *review)
		wf.Steps[0].Agent = *agentFlag
		if err := wf.Validate(); err != nil {
			return fail(stderr, err)
		}
	}

	if *noSandbox {
		for i := range wf.Steps {
			if wf.Steps[i].Kind() != model.StepCheck {
				wf.Steps[i].Sandbox.Mode = "none"
			}
		}
		fmt.Fprintln(stderr, "legatus: --no-sandbox: agents run without their own sandbox. They still work in this run's own worktree with a scrubbed environment, but can reach anything your user account can.")
	}

	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	if len(a.Pool.Snapshots()) == 0 {
		fmt.Fprintln(stderr, "legatus: no accounts yet. Add the login you already use with:\n\n  legatus accounts add --id main --provider codex --home default\n\nor start a separate one with `legatus accounts add --id second --provider codex` then `legatus accounts login second`.")
		return 1
	}
	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		return fail(stderr, err)
	}
	if !*quiet {
		a.Engine.OnEvent = func(ev model.Event) { printEvent(stdout, ev) }
	}
	a.Engine.RedactPII = *pii

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	run, err := a.Engine.NewRun(ctx, model.Task{Prompt: prompt, Repo: absRepo, Base: *base}, wf)
	if err != nil {
		return fail(stderr, err)
	}
	return finishExecution(ctx, a.Engine, run.ID, stdout, stderr, a.Store.Dir())
}

func cmdResume(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("resume", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "Usage: legatus resume <run id>")
		return 64
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	a.Engine.OnEvent = func(ev model.Event) { printEvent(stdout, ev) }
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return finishExecution(ctx, a.Engine, fs.Arg(0), stdout, stderr, a.Store.Dir())
}

// finishExecution runs (or resumes) a run and turns its final state into an exit code and a summary.
func finishExecution(ctx context.Context, e *engine.Engine, id string, stdout, stderr io.Writer, storeDir string) int {
	err := e.Resume(ctx, id)
	if errors.Is(err, context.Canceled) {
		fmt.Fprintf(stderr, "\nInterrupted. Nothing is lost; continue with:\n\n  legatus resume %s\n", id)
		return 130
	}
	if err != nil {
		return fail(stderr, err)
	}
	run, err := e.Store.Load(id)
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout)
	switch run.Status {
	case model.Succeeded:
		fmt.Fprintf(stdout, "Done. The work is on branch %s\n  worktree: %s\n  report:   %s\n", run.Branch, run.Worktree, filepath.Join(storeDir, "runs", run.ID, "evidence.md"))
		fmt.Fprintf(stdout, "  review it: git -C \"%s\" log --oneline %s..HEAD\n", run.Worktree, shortSHA(run.BaseCommit))
		return 0
	case model.NeedsHuman:
		fmt.Fprintf(stdout, "Needs a person: %s\n  branch %s, report %s\n", run.Error, run.Branch, filepath.Join(storeDir, "runs", run.ID, "evidence.md"))
		return 2
	default:
		fmt.Fprintf(stdout, "Failed: %s\n  report %s\n", run.Error, filepath.Join(storeDir, "runs", run.ID, "evidence.md"))
		return 1
	}
}

func cmdRuns(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("runs", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
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
	if len(runs) == 0 {
		fmt.Fprintln(stdout, "No runs yet. Try: legatus run \"your task\"")
		return 0
	}
	fmt.Fprintf(stdout, "%-9s %-17s %-10s %s\n", "RUN", "STATUS", "AGE", "TASK")
	for _, r := range runs {
		fmt.Fprintf(stdout, "%-9s %-17s %-10s %s\n", r.ID, strings.ReplaceAll(string(r.Status), "_", " "), age(r.CreatedAt), r.Task.Title)
	}
	return 0
}

func cmdShow(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("show", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	events := fs.Bool("events", false, "also print the journal")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "Usage: legatus show [-events] <run id>")
		return 64
	}
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	run, err := a.Store.Load(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	evs, _, _ := a.Store.Events(run.ID, 0)
	fmt.Fprint(stdout, engine.Evidence(run, evs, ""))
	if run.Status == model.WaitingCapacity {
		fmt.Fprintf(stdout, "\nWaiting for an account to reset until %s.\n", run.WaitUntil.Local().Format("Mon 15:04"))
	}
	if *events {
		fmt.Fprintln(stdout, "\n## Journal")
		for _, ev := range evs {
			printEvent(stdout, ev)
		}
	}
	return 0
}

func shortSHA(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// printEvent writes one journal event as a short human line. Chatty events are shortened or skipped.
func printEvent(w io.Writer, ev model.Event) {
	t := ev.Time.Local().Format("15:04:05")
	str := func(k string) string {
		if v, ok := ev.Data[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	step := ""
	if ev.Step != "" {
		step = "[" + ev.Step + "] "
	}
	var line string
	switch ev.Type {
	case "run.created":
		line = fmt.Sprintf("run %s created on branch %s", ev.Run, str("branch"))
		if r := str("redacted"); r != "" && r != "nothing redacted" {
			line += " (removed from the task: " + r + ")"
		}
	case "run.started", "step.started":
		if ev.Type == "step.started" {
			line = step + "started (" + str("type") + ")"
		} else {
			return
		}
	case "agent.started":
		line = step + "working as " + str("account") + " (" + str("provider") + ")"
		if i := str("independence"); i != "" {
			line += ", independent: " + i
		}
		if r := str("redacted"); r != "" && r != "nothing redacted" {
			line += ", removed from prompt: " + r
		}
	case "agent.message":
		line = step + "says: " + oneLine(str("text"), 110)
	case "agent.tool":
		line = step + "runs: " + oneLine(str("text"), 90)
	case "agent.usage", "agent.info":
		return
	case "agent.error":
		line = step + "agent error: " + oneLine(str("error"), 140)
	case "account.limited":
		line = step + "!! " + str("account") + " hit its usage limit; set aside until " + localTime(str("until")) + ". Continuing on another login."
	case "run.waiting_capacity":
		line = step + "!! every login is at its limit; waiting until " + localTime(str("until"))
	case "run.capacity_back":
		line = step + "a login is available again; continuing"
	case "check.started":
		line = step + "check: " + str("command")
	case "check.passed":
		line = step + "check passed"
	case "check.failed":
		line = step + "check FAILED (exit " + str("exit_code") + ")"
	case "review.verdict":
		line = step + "review: " + str("verdict") + " by " + str("account") + " (" + str("independence") + "): " + oneLine(str("summary"), 100)
	case "review.no_verdict":
		line = step + "the reviewer did not give a verdict"
	case "step.succeeded":
		line = step + "done. " + oneLine(str("summary"), 100)
	case "step.sent_back":
		line = step + "sent back to " + str("to") + " (" + str("times") + "x): " + oneLine(str("reason"), 100)
	case "run.resumed":
		line = "resuming from step " + str("from_step")
	case "run.committed":
		return
	case "run.succeeded":
		line = "run succeeded"
	case "run.needs_human":
		line = "needs a person: " + oneLine(str("reason"), 140)
	case "run.failed":
		line = "run FAILED: " + oneLine(str("error"), 140)
	default:
		return
	}
	fmt.Fprintf(w, "%s  %s\n", t, line)
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max-1] + "…"
	}
	return s
}

func localTime(rfc string) string {
	if t, err := time.Parse(time.RFC3339Nano, rfc); err == nil {
		return t.Local().Format("Mon 15:04")
	}
	return rfc
}
