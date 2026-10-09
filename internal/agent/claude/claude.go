// Package claude drives Claude Code in headless mode (`claude -p --output-format stream-json`). Each
// Legatus account is a separate CLAUDE_CONFIG_DIR, so several logins can run side by side.
//
// STATUS: run for real against Claude Code 2.1.295 on Linux (a plain reply and a run that used the Bash
// tool; their output is kept in testdata/ and read by the tests). A real usage limit has not been caught:
// the limit path is built from the messages and the rate_limit_event that version prints, which were read
// from the program, not provoked. Other versions and Windows and macOS are untried; `legatus doctor` says so.
//
// Limits of what it can enforce: the read-only mode allows only the Read, Grep and Glob tools, and the
// network setting only blocks Claude's own web tools. A shell command the agent is allowed to run can
// still use the network; use the codex backend or a container when that matters.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/runner"
	"github.com/Mvnshi/legatus/internal/pool"
)

// VerifiedVersion is the Claude Code release the backend has been run against for real (on Linux).
const VerifiedVersion = "2.1.295"

// Backend runs Claude Code.
type Backend struct {
	Exe        string // the claude executable; "claude" when empty
	PrefixArgs []string
	Model      string
	ExtraArgs  []string
	Now        func() time.Time
}

func (b *Backend) Provider() string { return "claude" }

func (b *Backend) exe() string {
	if b.Exe != "" {
		return b.Exe
	}
	return Executable()
}

// Executable is the claude program to run: $LEGATUS_CLAUDE_PATH when set (for installs that are not on PATH), else "claude".
func Executable() string {
	if v := os.Getenv("LEGATUS_CLAUDE_PATH"); v != "" {
		return v
	}
	return "claude"
}

func (b *Backend) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// AccountEnv points Claude Code at the login's own configuration directory.
func (b *Backend) AccountEnv(a pool.Account) map[string]string {
	if a.Home == "" {
		return nil
	}
	return map[string]string{"CLAUDE_CONFIG_DIR": a.Home}
}

type event struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
	// api_error_status is the HTTP status of the API's refusal on a "result" (429 when the login is rate limited).
	APIErrorStatus int `json:"api_error_status"`
	Message        *struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
	Usage        map[string]any `json:"usage"`
	TotalCostUSD float64        `json:"total_cost_usd"`
	// rate_limit_event: Claude Code reports the state of the login's usage windows as it goes.
	RateLimit *rateLimit `json:"rate_limit_info"`
}

type rateLimit struct {
	Status          string `json:"status"` // allowed, allowed_warning or rejected
	ResetsAt        int64  `json:"resetsAt"`
	RateLimitType   string `json:"rateLimitType"` // five_hour, seven_day, ...
	OverageStatus   string `json:"overageStatus"`
	OverageResetsAt int64  `json:"overageResetsAt"`
}

// resetAt is when a rejected login can run again: the window's reset, or the overage's if that comes first
// (Claude Code shows the earlier of the two). Zero when the event gives no time.
func (r *rateLimit) resetAt() time.Time {
	at := r.ResetsAt
	if r.OverageStatus == "rejected" && r.OverageResetsAt > 0 && (at <= 0 || r.OverageResetsAt < at) {
		at = r.OverageResetsAt
	}
	if at <= 0 {
		return time.Time{}
	}
	return time.Unix(at, 0)
}

// Args builds the command line. The prompt goes through stdin.
func (b *Backend) Args(req agent.Request) []string {
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if req.Sandbox.ReadOnly {
		args = append(args, "--permission-mode", "default", "--allowedTools", "Read,Grep,Glob",
			"--disallowedTools", "Edit,Write,Bash,WebFetch,WebSearch")
	} else {
		args = append(args, "--permission-mode", "acceptEdits", "--allowedTools", "Read,Grep,Glob,Edit,Write,Bash")
		if !req.Sandbox.Network {
			args = append(args, "--disallowedTools", "WebFetch,WebSearch")
		}
	}
	model := b.Model
	if req.Account.Model != "" {
		model = req.Account.Model
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return append(args, b.ExtraArgs...)
}

func (b *Backend) Run(ctx context.Context, req agent.Request, emit func(agent.Event)) (agent.Result, error) {
	var (
		final    *event
		lastText string
		rejected *rateLimit // the last rate_limit_event that said the login was refused
	)
	res, err := runner.Stream(ctx, runner.Spec{
		Exe: b.exe(), PrefixArgs: b.PrefixArgs, Args: b.Args(req), Dir: req.Dir, Env: req.Env, Stdin: req.Prompt,
	}, func(line []byte) {
		var ev event
		if json.Unmarshal(line, &ev) != nil {
			return
		}
		switch ev.Type {
		case "assistant":
			if ev.Message == nil {
				return
			}
			for _, c := range ev.Message.Content {
				switch c.Type {
				case "text":
					lastText = c.Text
					emit(agent.Event{Kind: agent.Message, Text: c.Text})
				case "tool_use":
					emit(agent.Event{Kind: agent.Tool, Text: c.Name, Data: map[string]any{"input": clip(string(c.Input), 500)}})
				}
			}
		case "rate_limit_event":
			if ev.RateLimit != nil && ev.RateLimit.Status == "rejected" {
				r := *ev.RateLimit
				rejected = &r
			}
		case "result":
			e := ev
			final = &e
			emit(agent.Event{Kind: agent.Usage, Data: map[string]any{"usage": ev.Usage, "cost_usd": ev.TotalCostUSD}})
		}
	})
	if err != nil {
		return agent.Result{}, err
	}

	failed := res.ExitCode != 0 || final == nil || final.IsError
	if failed {
		msg := ""
		switch {
		case final != nil && strings.TrimSpace(final.Result) != "":
			msg = strings.TrimSpace(final.Result)
		case strings.TrimSpace(res.StderrTail) != "":
			msg = strings.TrimSpace(res.StderrTail)
		case final == nil:
			msg = fmt.Sprintf("claude ended without a result (exit code %d)", res.ExitCode)
		default:
			msg = "claude reported an error: " + final.Subtype
		}
		// A refusal that Claude Code reported as an event is the surest sign, and it carries the reset as a
		// timestamp. The wording of the message is the fallback.
		if rejected != nil {
			if rejected.RateLimitType != "" && !agent.IsLimitMessage(msg) {
				msg = fmt.Sprintf("Claude Code reports the %s usage limit is reached: %s", rejected.RateLimitType, msg)
			}
			return agent.Result{}, &agent.LimitError{ResetAt: rejected.resetAt(), Message: msg}
		}
		refused := final != nil && final.APIErrorStatus == http.StatusTooManyRequests && !agent.IsServerThrottle(msg)
		if refused || agent.IsLimitMessage(msg) || agent.IsLimitMessage(res.StderrTail) {
			return agent.Result{}, &agent.LimitError{ResetAt: agent.ParseReset(msg+" "+res.StderrTail, b.now()), Message: msg}
		}
		return agent.Result{}, errors.New("claude failed: " + clip(msg, 1500))
	}
	summary := strings.TrimSpace(final.Result)
	if summary == "" {
		summary = strings.TrimSpace(lastText)
	}
	if summary == "" {
		summary = "(the agent returned no message)"
	}
	return agent.Result{Summary: summary}, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
