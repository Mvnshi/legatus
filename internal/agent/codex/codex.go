// Package codex drives the Codex command line (`codex exec --json`). Each Legatus account is a separate
// CODEX_HOME, so several ChatGPT logins can run side by side.
//
// The event shapes below were captured from codex-cli 0.162; error and limit shapes are parsed
// tolerantly because they could not be provoked on demand.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/runner"
	"github.com/Mvnshi/legatus/internal/pool"
)

// Backend runs Codex.
type Backend struct {
	Exe        string   // the codex executable; "codex" when empty
	PrefixArgs []string // for tests
	Model      string   // passed as -m when set
	ExtraArgs  []string // extra `codex exec` options, placed before the prompt
	Now        func() time.Time
	GOOS       string // the operating system; runtime.GOOS when empty (tests set it)
}

func (b *Backend) Provider() string { return "codex" }

func (b *Backend) exe() string {
	if b.Exe != "" {
		return b.Exe
	}
	return Executable()
}

// Executable is the codex program to run: $LEGATUS_CODEX_PATH when set (for installs that are not on PATH), else "codex".
func Executable() string {
	if v := os.Getenv("LEGATUS_CODEX_PATH"); v != "" {
		return v
	}
	return "codex"
}

func (b *Backend) goos() string {
	if b.GOOS != "" {
		return b.GOOS
	}
	return runtime.GOOS
}

var windowsSandboxRE = regexp.MustCompile(`(?m)^\[windows\][^\[]*?^\s*sandbox\s*=\s*["']([a-z]+)["']`)

// WindowsSandboxMode is the Windows sandbox mode ("elevated" or "unelevated") set in the login's
// config.toml, or "unelevated" (which needs no one-time administrator setup) when it sets none. home is
// the login's CODEX_HOME; empty means Codex's default, ~/.codex.
func WindowsSandboxMode(home string) string {
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, ".codex")
		}
	}
	if data, err := os.ReadFile(filepath.Join(home, "config.toml")); err == nil {
		if m := windowsSandboxRE.FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	return "unelevated"
}

func (b *Backend) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// AccountEnv points Codex at the login's own home directory.
func (b *Backend) AccountEnv(a pool.Account) map[string]string {
	if a.Home == "" {
		return nil
	}
	return map[string]string{"CODEX_HOME": a.Home}
}

type event struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
	Item *struct {
		Type             string `json:"type"`
		Text             string `json:"text"`
		Command          string `json:"command"`
		ExitCode         *int   `json:"exit_code"`
		AggregatedOutput string `json:"aggregated_output"`
		Message          string `json:"message"`
	} `json:"item"`
	Usage map[string]any `json:"usage"`
}

// Args builds the `codex exec` command line for a request. The prompt itself goes through stdin.
func (b *Backend) Args(req agent.Request, lastMessageFile string) []string {
	args := []string{"exec", "--json", "--ephemeral", "--skip-git-repo-check", "--ignore-user-config", "-C", req.Dir, "-o", lastMessageFile}
	if b.goos() == "windows" && !req.Sandbox.Unrestricted {
		// --ignore-user-config also drops the Windows sandbox mode the login set up (without it the
		// workspace is read-only), so carry that one setting over.
		args = append(args, "-c", fmt.Sprintf("windows.sandbox=%q", WindowsSandboxMode(req.Account.Home)))
	}
	if req.Sandbox.Unrestricted {
		// The user asked for no operating-system sandbox. The agent still works in the run's worktree.
		args = append(args, "--sandbox", "danger-full-access")
	} else if req.Sandbox.ReadOnly {
		args = append(args, "--sandbox", "read-only")
	} else {
		args = append(args, "--sandbox", "workspace-write", "-c", fmt.Sprintf("sandbox_workspace_write.network_access=%t", req.Sandbox.Network))
	}
	model := b.Model
	if req.Account.Model != "" {
		model = req.Account.Model // the login's own choice is the more specific one
	}
	if model != "" {
		args = append(args, "-m", model)
	}
	if req.Account.Effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", req.Account.Effort))
	}
	args = append(args, b.ExtraArgs...)
	return append(args, "-")
}

// Preflight checks that Codex's own sandbox can start a process for this login. It runs `codex sandbox`,
// which needs no model. Only Windows is probed: there the sandbox depends on one-time setup that can be
// missing or broken (a model turn then hangs rather than failing); macOS and Linux use the system's
// built-in sandboxing.
//
// A passing probe has not been observed on a machine where Codex's Windows sandbox works (it was written
// on one where the probe fails), so treat "passes" as "not known to be broken".
func (b *Backend) Preflight(ctx context.Context, a pool.Account, dir string, env []string) error {
	if b.goos() != "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var output []string
	res, err := runner.Stream(ctx, runner.Spec{
		Exe: b.exe(), PrefixArgs: b.PrefixArgs,
		Args: []string{"sandbox", "-c", fmt.Sprintf("windows.sandbox=%q", WindowsSandboxMode(a.Home)), "cmd", "/c", "echo legatus-sandbox-ok"},
		Dir:  dir, Env: env,
	}, func(line []byte) { output = append(output, string(line)) })
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return errors.New("Codex's Windows sandbox did not answer within 60 seconds")
		}
		return err
	}
	if res.ExitCode != 0 || !strings.Contains(strings.Join(output, "\n"), "legatus-sandbox-ok") {
		detail := strings.TrimSpace(res.StderrTail + " " + strings.Join(output, " "))
		if detail == "" {
			detail = fmt.Sprintf("exit code %d", res.ExitCode)
		}
		return errors.New("Codex's Windows sandbox cannot start a process: " + clip(detail, 400))
	}
	return nil
}

func (b *Backend) Run(ctx context.Context, req agent.Request, emit func(agent.Event)) (agent.Result, error) {
	dir, err := os.MkdirTemp("", "legatus-codex-")
	if err != nil {
		return agent.Result{}, err
	}
	defer os.RemoveAll(dir)
	lastFile := filepath.Join(dir, "last-message.txt")

	var (
		lastMessage    string
		failure        string // the message of a turn.failed or error event
		turnFailed     bool
		turnCompleted  bool
		firstErrorText string
	)
	res, err := runner.Stream(ctx, runner.Spec{
		Exe: b.exe(), PrefixArgs: b.PrefixArgs, Args: b.Args(req, lastFile), Dir: req.Dir, Env: req.Env, Stdin: req.Prompt,
	}, func(line []byte) {
		var ev event
		if json.Unmarshal(line, &ev) != nil {
			return // not JSON: a banner or a warning
		}
		switch ev.Type {
		case "item.started", "item.completed":
			if ev.Item == nil {
				return
			}
			switch ev.Item.Type {
			case "agent_message":
				if ev.Type == "item.completed" {
					lastMessage = ev.Item.Text
					emit(agent.Event{Kind: agent.Message, Text: ev.Item.Text})
				}
			case "command_execution":
				data := map[string]any{"command": ev.Item.Command}
				if ev.Item.ExitCode != nil {
					data["exit_code"] = *ev.Item.ExitCode
				}
				emit(agent.Event{Kind: agent.Tool, Text: ev.Item.Command, Data: data})
			case "file_change", "mcp_tool_call", "web_search":
				emit(agent.Event{Kind: agent.Tool, Text: ev.Item.Type})
			case "error":
				if ev.Item.Message != "" && firstErrorText == "" {
					firstErrorText = ev.Item.Message
				}
			}
		case "turn.completed":
			turnCompleted = true
			emit(agent.Event{Kind: agent.Usage, Data: ev.Usage})
		case "turn.failed":
			turnFailed = true
			if ev.Error != nil {
				failure = ev.Error.Message
			}
		case "error":
			// Codex also reports recoverable problems ("Reconnecting...") as error events, so on their
			// own they do not fail the run.
			if ev.Message != "" {
				firstErrorText = ev.Message
			}
		}
	})
	if err != nil {
		return agent.Result{}, err
	}

	fatal := turnFailed || (res.ExitCode != 0 && !turnCompleted)
	if fatal {
		msg := strings.TrimSpace(failure)
		if msg == "" {
			msg = strings.TrimSpace(firstErrorText)
		}
		if msg == "" {
			msg = strings.TrimSpace(res.StderrTail)
		}
		if msg == "" {
			msg = fmt.Sprintf("codex exited with code %d", res.ExitCode)
		}
		if agent.IsLimitMessage(msg) || agent.IsLimitMessage(res.StderrTail) {
			return agent.Result{}, &agent.LimitError{ResetAt: agent.ParseReset(msg+" "+res.StderrTail, b.now()), Message: msg}
		}
		return agent.Result{}, errors.New("codex failed: " + clip(msg, 1500))
	}
	summary := lastMessage
	if data, err := os.ReadFile(lastFile); err == nil && strings.TrimSpace(string(data)) != "" {
		summary = strings.TrimSpace(string(data))
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
