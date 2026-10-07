// Package claude drives Claude Code in headless mode (`claude -p --output-format stream-json`). Each
// Legatus account is a separate CLAUDE_CONFIG_DIR, so several logins can run side by side.
//
// STATUS: written from Claude Code's documented headless interface and tested against scripted output
// only. It has not been run against a real Claude Code install (none is available on the machine this was
// written on), so `legatus doctor` reports it as unverified until someone runs it for real.
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
	"os"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/runner"
	"github.com/Mvnshi/legatus/internal/pool"
)

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
	Message *struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
	Usage        map[string]any `json:"usage"`
	TotalCostUSD float64        `json:"total_cost_usd"`
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
	if b.Model != "" {
		args = append(args, "--model", b.Model)
	}
	return append(args, b.ExtraArgs...)
}

func (b *Backend) Run(ctx context.Context, req agent.Request, emit func(agent.Event)) (agent.Result, error) {
	var (
		final    *event
		lastText string
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
		if agent.IsLimitMessage(msg) || agent.IsLimitMessage(res.StderrTail) {
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
