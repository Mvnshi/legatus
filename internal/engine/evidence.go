package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/workflow"
)

// writeEvidence saves evidence.md next to the run: what was asked, what ran on which login, what the
// checks and the reviewer said, and which limits were hit. It is what a person reads before merging.
func (e *Engine) writeEvidence(ctx context.Context, run *model.Run, wf *workflow.Workflow) error {
	events, _, err := e.Store.Events(run.ID, 0)
	if err != nil {
		return err
	}
	stat, _ := e.Worktrees.DiffStat(ctx, run.Worktree, run.BaseCommit)
	dir, err := e.Store.RunDir(run.ID)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "evidence.md"), []byte(Evidence(run, events, stat)), 0o600)
}

// Evidence renders the report for a run. It has no side effects.
func Evidence(run *model.Run, events []model.Event, diffStat string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", run.Task.Title)
	fmt.Fprintf(&b, "- Run `%s`, **%s**\n", run.ID, strings.ReplaceAll(string(run.Status), "_", " "))
	fmt.Fprintf(&b, "- Branch `%s`, started from `%s`\n", run.Branch, shortSHA(run.BaseCommit))
	if run.Error != "" {
		fmt.Fprintf(&b, "- Stopped because: %s\n", run.Error)
	}
	b.WriteString("\n## Task\n\n" + strings.TrimSpace(run.Task.Prompt) + "\n")

	b.WriteString("\n## Steps\n\n| Step | Type | Result | Login | Attempts | Detail |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, s := range run.Steps {
		login := s.Account
		if login == "" {
			login = "-"
		}
		detail := s.Summary
		if s.Error != "" {
			detail = s.Error
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %s |\n", s.ID, s.Kind, s.Status, login, s.Attempts, cell(detail))
	}
	if len(run.Retries) > 0 {
		b.WriteString("\nSent back for another try: ")
		var parts []string
		for _, s := range run.Steps {
			if n := run.Retries[s.ID]; n > 0 {
				parts = append(parts, fmt.Sprintf("%s ×%d", s.ID, n))
			}
		}
		b.WriteString(strings.Join(parts, ", ") + "\n")
	}

	var limits, reviews, redactions []string
	for _, ev := range events {
		switch ev.Type {
		case "account.limited":
			limits = append(limits, fmt.Sprintf("- `%v` (%v) hit its usage limit during `%s`; set aside until %v", ev.Data["account"], ev.Data["provider"], ev.Step, formatUntil(ev.Data["until"])))
		case "review.verdict":
			line := fmt.Sprintf("- `%s`: **%v** by `%v` (%v): %v", ev.Step, ev.Data["verdict"], ev.Data["account"], ev.Data["independence"], ev.Data["summary"])
			if issues, ok := ev.Data["issues"].([]any); ok {
				for _, issue := range issues {
					line += fmt.Sprintf("\n  - %v", issue)
				}
			}
			reviews = append(reviews, line)
		case "agent.started":
			if r, _ := ev.Data["redacted"].(string); r != "" && r != "nothing redacted" {
				redactions = append(redactions, fmt.Sprintf("- `%s` attempt %v: %s", ev.Step, ev.Data["attempt"], r))
			}
		}
	}
	section := func(title string, lines []string, none string) {
		b.WriteString("\n## " + title + "\n\n")
		if len(lines) == 0 {
			b.WriteString(none + "\n")
			return
		}
		b.WriteString(strings.Join(lines, "\n") + "\n")
	}
	section("Usage limits", limits, "None were hit.")
	section("Review", reviews, "No independent review ran.")
	section("Secrets removed from prompts", redactions, "Nothing needed removing.")

	b.WriteString("\n## Changes\n\n")
	if strings.TrimSpace(diffStat) == "" {
		b.WriteString("No files changed.\n")
	} else {
		b.WriteString("```\n" + strings.TrimSpace(diffStat) + "\n```\n")
	}
	return b.String()
}

func cell(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

func shortSHA(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func formatUntil(v any) string {
	text := fmt.Sprint(v)
	if t, err := time.Parse(time.RFC3339, text); err == nil {
		return t.UTC().Format("2006-01-02 15:04 UTC")
	}
	return text
}
