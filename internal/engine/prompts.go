package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/workflow"
)

const rules = `Rules: work only inside this directory. Do not push, switch branches or rewrite history; Legatus commits your work. When you are done, reply with a short summary of what you changed and anything you could not do.`

// implementPrompt is what the coding agent is asked to do. After an interruption or a failed check it
// also says what happened, so the work continues instead of starting over.
func (e *Engine) implementPrompt(ctx context.Context, run *model.Run, step workflow.Step) string {
	if step.Prompt != "" {
		stat, _ := e.Worktrees.DiffStat(ctx, run.Worktree, run.BaseCommit)
		return strings.NewReplacer(
			"{{task}}", run.Task.Prompt,
			"{{failure}}", run.Feedback,
			"{{handoff}}", run.Handoff,
			"{{diff}}", strings.TrimSpace(stat),
		).Replace(step.Prompt)
	}
	var b strings.Builder
	b.WriteString("You are working in a git worktree on this task:\n\n")
	b.WriteString(strings.TrimSpace(run.Task.Prompt))
	b.WriteString("\n")
	if run.Handoff != "" {
		b.WriteString("\n" + run.Handoff + "\nContinue from there. Do not start over.\n")
	}
	if run.Feedback != "" {
		b.WriteString("\nThe last attempt was sent back. Fix this:\n" + strings.TrimSpace(run.Feedback) + "\n")
	}
	b.WriteString("\n" + rules + "\n")
	return b.String()
}

const maxReviewDiff = 60_000

// reviewPrompt asks for an independent judgement of the change, ending in a JSON verdict.
func (e *Engine) reviewPrompt(ctx context.Context, run *model.Run, wf *workflow.Workflow) (string, error) {
	diff, err := e.Worktrees.Diff(ctx, run.Worktree, run.BaseCommit, maxReviewDiff)
	if err != nil {
		return "", fmt.Errorf("could not read the change to review: %w", err)
	}
	var b strings.Builder
	b.WriteString("You are an independent code reviewer. You did not write this change and must not modify any file.\n\nThe task was:\n\n")
	b.WriteString(strings.TrimSpace(run.Task.Prompt))
	b.WriteString("\n\nThe change (a diff against where the work started):\n\n")
	if strings.TrimSpace(diff) == "" {
		b.WriteString("(the diff is empty: nothing was changed)\n")
	} else {
		b.WriteString(diff)
	}
	var checks []string
	for i, s := range wf.Steps {
		if s.Kind() == model.StepCheck && run.Steps[i].Summary != "" {
			checks = append(checks, "- "+s.ID+": "+run.Steps[i].Summary)
		}
	}
	if len(checks) > 0 {
		b.WriteString("\nAutomated checks that ran:\n" + strings.Join(checks, "\n") + "\n")
	}
	b.WriteString(`
Review it against the task: is it correct, complete, and free of problems you would block a pull request for? Give a short review, then end your reply with exactly one JSON object on its own line:
{"verdict":"approve" or "request_changes","summary":"one sentence","issues":["each problem to fix"]}
Approve only if there is nothing you would ask to change. An empty diff is never an approval of a task that asked for changes.
`)
	return b.String(), nil
}

type verdict struct {
	Verdict string   `json:"verdict"`
	Summary string   `json:"summary"`
	Issues  []string `json:"issues"`
}

// parseVerdict finds the last JSON object in the reviewer's reply that has a usable verdict. Anything
// else (no JSON, an unknown verdict) is "no verdict", never an approval.
func parseVerdict(reply string) (verdict, bool) {
	if len(reply) > 20_000 {
		reply = reply[len(reply)-20_000:]
	}
	for i := len(reply) - 1; i >= 0; i-- {
		if reply[i] != '{' {
			continue
		}
		var v verdict
		if json.NewDecoder(strings.NewReader(reply[i:])).Decode(&v) != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(v.Verdict)) {
		case "approve", "approved":
			v.Verdict = "approve"
			return v, true
		case "request_changes", "request-changes", "changes_requested", "reject":
			v.Verdict = "request_changes"
			return v, true
		}
	}
	return verdict{}, false
}
