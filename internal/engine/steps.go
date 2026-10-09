package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/pool"
	"github.com/Mvnshi/legatus/internal/sandbox"
	"github.com/Mvnshi/legatus/internal/workflow"
)

const (
	defaultAgentTimeout = 30 * time.Minute
	defaultCheckTimeout = 10 * time.Minute
	defaultMaxAttempts  = 2
	outputTail          = 6000
)

// callSpec describes one agent call made on behalf of a step.
type callSpec struct {
	idx     int
	role    string // "implement" or "review"
	prompt  func() (string, error)
	sandbox agent.Sandbox
}

type callResult struct {
	res          agent.Result
	account      pool.Account
	independence string // for reviews: other-provider, other-account or same-account
}

// agentStep runs the coding agent and commits what it did.
func (e *Engine) agentStep(ctx context.Context, run *model.Run, wf *workflow.Workflow, idx int) (outcome, error) {
	step := wf.Steps[idx]
	cr, err := e.call(ctx, run, wf, callSpec{
		idx:  idx,
		role: "implement",
		prompt: func() (string, error) {
			return e.implementPrompt(ctx, run, step), nil
		},
		sandbox: agent.Sandbox{Network: step.Sandbox.Network, Unrestricted: step.Unsandboxed()},
	})
	if err != nil {
		return outcome{}, err
	}
	st := &run.Steps[idx]
	st.Summary = firstLine(cr.res.Summary, 400)
	// The work is done, so nothing about an earlier interruption or failure applies any more.
	run.Handoff, run.Feedback = "", ""
	if _, err := e.Worktrees.CommitAll(ctx, run.Worktree, "legatus: "+step.ID); err != nil {
		return outcome{}, fmt.Errorf("could not commit the agent's work: %w", err)
	}
	if !step.AllowEmpty {
		stat, err := e.Worktrees.DiffStat(ctx, run.Worktree, run.BaseCommit)
		if err != nil {
			return outcome{}, err
		}
		if strings.TrimSpace(stat) == "" {
			// An agent that could not do the work often still exits cleanly and says so in words.
			e.emit(run.ID, step.ID, "agent.no_changes", map[string]any{"said": clip(cr.res.Summary, 1500)})
			return outcome{kind: outFail, message: "the agent finished without changing any file. It said: " + firstLine(cr.res.Summary, 300)}, nil
		}
	}
	return outcome{kind: outOK}, nil
}

// reviewStep asks an independent agent to judge the change, read-only.
func (e *Engine) reviewStep(ctx context.Context, run *model.Run, wf *workflow.Workflow, idx int) (outcome, error) {
	step := wf.Steps[idx]
	cr, err := e.call(ctx, run, wf, callSpec{
		idx:  idx,
		role: "review",
		prompt: func() (string, error) {
			return e.reviewPrompt(ctx, run, wf)
		},
		sandbox: agent.Sandbox{ReadOnly: true, Unrestricted: step.Unsandboxed()},
	})
	if err != nil {
		return outcome{}, err
	}
	st := &run.Steps[idx]
	reverted := false
	// A reviewer judges the change; it never gets to alter it. Whatever it touched is reverted, whether or
	// not the agent's own sandbox was on.
	if status, _ := e.Worktrees.Status(ctx, run.Worktree); strings.TrimSpace(status) != "" {
		e.emit(run.ID, step.ID, "review.modified_files", map[string]any{"status": clip(status, 1000)})
		if err := e.Worktrees.Reset(ctx, run.Worktree); err != nil {
			return outcome{}, fmt.Errorf("the reviewer changed files and they could not be reverted: %w", err)
		}
		reverted = true
	}
	v, ok := parseVerdict(cr.res.Summary)
	if !ok {
		st.Summary = "the reviewer did not return a verdict"
		e.emit(run.ID, step.ID, "review.no_verdict", map[string]any{"output": clip(cr.res.Summary, 1500)})
		return outcome{kind: outHuman, message: "the reviewer did not return a verdict (" + cr.independence + "); a person should look at the change"}, nil
	}
	note := ""
	if reverted {
		note = "; it changed files and they were reverted"
	}
	st.Summary = fmt.Sprintf("%s (%s%s): %s", v.Verdict, cr.independence, note, firstLine(v.Summary, 300))
	e.emit(run.ID, step.ID, "review.verdict", map[string]any{
		"verdict": v.Verdict, "summary": v.Summary, "issues": v.Issues,
		"independence": cr.independence, "account": cr.account.ID, "provider": cr.account.Provider,
	})
	if v.Verdict == "approve" {
		return outcome{kind: outOK}, nil
	}
	feedback := "An independent reviewer asked for changes: " + v.Summary
	for _, issue := range v.Issues {
		feedback += "\n- " + issue
	}
	return e.sendBackOrStop(run, wf, idx, feedback, "the reviewer asked for changes: "+firstLine(v.Summary, 200), outHuman), nil
}

// checkStep runs shell commands in the worktree; the first failure stops the step.
func (e *Engine) checkStep(ctx context.Context, run *model.Run, wf *workflow.Workflow, idx int) (outcome, error) {
	step := wf.Steps[idx]
	st := &run.Steps[idx]
	st.Attempts++
	timeout := time.Duration(step.Timeout)
	if timeout <= 0 {
		timeout = e.CheckTimeout
	}
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	env := sandbox.Env(e.environ(), append([]string{"GOPATH", "GOCACHE", "GOMODCACHE", "GOFLAGS", "CI"}, step.Sandbox.Env...), nil)
	for _, command := range step.Run {
		e.emit(run.ID, step.ID, "check.started", map[string]any{"command": command})
		release, gerr := e.acquireCheck(ctx, run.ID, step.ID)
		if gerr != nil {
			return outcome{}, gerr
		}
		shell := e.shell
		if shell == nil {
			shell = runShell
		}
		output, code, err := shell(ctx, run.Worktree, command, env, timeout)
		release()
		if ctx.Err() != nil {
			return outcome{}, ctx.Err()
		}
		if err == nil && code == 0 {
			e.emit(run.ID, step.ID, "check.passed", map[string]any{"command": command})
			continue
		}
		reason := fmt.Sprintf("`%s` failed", command)
		if code > 0 {
			reason = fmt.Sprintf("`%s` failed (exit code %d)", command, code)
		}
		if err != nil && code <= 0 {
			reason += ": " + err.Error()
		}
		st.Summary = reason
		e.emit(run.ID, step.ID, "check.failed", map[string]any{"command": command, "exit_code": code, "output": output})
		feedback := reason + ". The end of its output was:\n" + output
		return e.sendBackOrStop(run, wf, idx, feedback, reason, outFail), nil
	}
	st.Summary = fmt.Sprintf("%d command(s) passed", len(step.Run))
	return outcome{kind: outOK}, nil
}

// sendBackOrStop returns the run to the step named in on_fail while that has not been done too often;
// otherwise it ends with the given outcome (fail for checks, hand to a person for reviews).
func (e *Engine) sendBackOrStop(run *model.Run, wf *workflow.Workflow, idx int, feedback, reason string, stop outcomeKind) outcome {
	step := wf.Steps[idx]
	if step.OnFail != nil && run.Retries[step.ID] < step.OnFail.Max {
		return outcome{kind: outRetry, retryTo: wf.StepIndex(step.OnFail.RetryWith), feedback: feedback, message: reason}
	}
	msg := reason
	if step.OnFail != nil && step.OnFail.Max > 0 {
		msg += fmt.Sprintf(" (sent back %d time(s) already)", run.Retries[step.ID])
	}
	return outcome{kind: stop, message: msg}
}

// call runs one agent request for a step, handling usage limits: the account is set aside, what the
// interrupted attempt achieved is committed and described, and the work continues on another account,
// or the run waits for the earliest reset when none is left.
func (e *Engine) call(ctx context.Context, run *model.Run, wf *workflow.Workflow, spec callSpec) (callResult, error) {
	step := wf.Steps[spec.idx]
	st := &run.Steps[spec.idx]
	maxAttempts := step.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	failures := 0
	for {
		lease, independence, err := e.lease(ctx, run, wf, spec.idx)
		var capacity *pool.CapacityError
		switch {
		case errors.As(err, &capacity):
			if err := e.park(ctx, run, step.ID, capacity.Until); err != nil {
				return callResult{}, err
			}
			continue
		case errors.Is(err, pool.ErrNoAccounts):
			return callResult{}, fmt.Errorf("no account can run step %q (agent: %s); add one with `legatus accounts add`", step.ID, step.Agent)
		case err != nil:
			return callResult{}, err
		}
		backend, ok := e.Backends[lease.Account.Provider]
		if !ok {
			lease.Release()
			return callResult{}, fmt.Errorf("account %q uses provider %q, which this build has no backend for", lease.Account.ID, lease.Account.Provider)
		}
		if !spec.sandbox.Unrestricted {
			if err := e.checkSandbox(ctx, run, backend, lease.Account, step); err != nil {
				lease.Release()
				return callResult{}, err
			}
		}
		st.Account, st.Provider = lease.Account.ID, lease.Account.Provider
		st.Attempts++
		if err := e.save(run); err != nil {
			lease.Release()
			return callResult{}, err
		}
		prompt, err := spec.prompt()
		if err != nil {
			lease.Release()
			return callResult{}, err
		}
		prompt, findings := sandbox.Redact(prompt, e.RedactPII)
		e.emit(run.ID, step.ID, "agent.started", map[string]any{
			"account": lease.Account.ID, "provider": lease.Account.Provider, "attempt": st.Attempts,
			"role": spec.role, "independence": independence, "redacted": sandbox.Summary(findings),
		})
		timeout := time.Duration(step.Timeout)
		if timeout <= 0 {
			timeout = e.AgentTimeout
		}
		if timeout <= 0 {
			timeout = defaultAgentTimeout
		}
		req := agent.Request{
			Dir: run.Worktree, Prompt: prompt, Account: lease.Account, Sandbox: spec.sandbox,
			Timeout: timeout, Role: spec.role,
			Env: sandbox.Env(e.environ(), step.Sandbox.Env, backend.AccountEnv(lease.Account)),
		}
		actx, cancel := context.WithTimeout(ctx, timeout)
		res, err := backend.Run(actx, req, func(ev agent.Event) {
			e.emit(run.ID, step.ID, "agent."+string(ev.Kind), map[string]any{"text": clip(ev.Text, 2000), "data": ev.Data})
		})
		cancel()

		var limit *agent.LimitError
		switch {
		case err == nil:
			lease.Release()
			return callResult{res: res, account: lease.Account, independence: independence}, nil
		case errors.As(err, &limit):
			until := lease.ReportLimit(limit.ResetAt)
			e.emit(run.ID, step.ID, "account.limited", map[string]any{
				"account": lease.Account.ID, "provider": lease.Account.Provider, "until": until, "message": clip(limit.Message, 300),
			})
			e.noteInterrupted(ctx, run, step, lease.Account)
			continue
		case ctx.Err() != nil:
			lease.Release()
			return callResult{}, ctx.Err()
		default:
			lease.Release()
			failures++
			e.emit(run.ID, step.ID, "agent.error", map[string]any{"account": lease.Account.ID, "error": clip(err.Error(), 1000), "failures": failures})
			if failures >= maxAttempts {
				return callResult{}, fmt.Errorf("step %q failed %d times; last error: %w", step.ID, failures, err)
			}
			if spec.role == "implement" {
				run.Feedback = "The previous attempt ended with an error: " + clip(err.Error(), 800)
			}
		}
	}
}

// checkSandbox asks a backend that can check whether the agent's sandbox works for this login. The answer
// is remembered for the life of the process, so a broken sandbox is reported once, quickly, instead of
// every agent run hanging.
func (e *Engine) checkSandbox(ctx context.Context, run *model.Run, backend agent.Backend, account pool.Account, step workflow.Step) error {
	pf, ok := backend.(agent.Preflighter)
	if !ok {
		return nil
	}
	e.pfMu.Lock()
	defer e.pfMu.Unlock()
	if e.pfDone == nil {
		e.pfDone = map[string]error{}
	}
	err, seen := e.pfDone[account.ID]
	if !seen {
		env := sandbox.Env(e.environ(), step.Sandbox.Env, backend.AccountEnv(account))
		err = pf.Preflight(ctx, account, run.Worktree, env)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		e.pfDone[account.ID] = err
		e.emit(run.ID, step.ID, "sandbox.checked", map[string]any{"account": account.ID, "ok": err == nil, "detail": errText(err)})
	}
	if err != nil {
		return &SandboxError{Account: account.ID, Cause: err}
	}
	return nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// SandboxError means the agent's own sandbox cannot run for an account, and the user has not allowed
// running without it.
type SandboxError struct {
	Account string
	Cause   error
}

func (e *SandboxError) Error() string {
	return fmt.Sprintf("the sandbox for account %q does not work: %v. Legatus will not run agents without a sandbox unless you say so: "+
		"run with --no-sandbox (or set `sandbox: {mode: none}` on the step) to let the agent work with the full access of your user account, "+
		"still inside the run's own worktree and with a scrubbed environment", e.Account, e.Cause)
}

func (e *SandboxError) Unwrap() error { return e.Cause }

// lease picks an account for the step. A review asks for an independent one: another provider if there
// is one, else another login of the same provider, else (and only then) the author's own login.
func (e *Engine) lease(ctx context.Context, run *model.Run, wf *workflow.Workflow, idx int) (*pool.Lease, string, error) {
	step := wf.Steps[idx]
	if ref, ok := strings.CutPrefix(step.Agent, "other-than:"); ok {
		author := run.Steps[wf.StepIndex(ref)]
		tiers := []struct {
			f     pool.Filter
			label string
		}{
			{pool.Filter{ExcludeProvider: author.Provider}, "other-provider"},
			{pool.Filter{ExcludeAccounts: map[string]bool{author.Account: true}}, "other-account"},
			{pool.Filter{}, "same-account"},
		}
		var earliest *pool.CapacityError
		for _, t := range tiers {
			l, err := e.Pool.Acquire(ctx, t.f)
			var ce *pool.CapacityError
			switch {
			case err == nil:
				return l, t.label, nil
			case errors.As(err, &ce):
				if earliest == nil || ce.Until.Before(earliest.Until) {
					earliest = ce
				}
			case errors.Is(err, pool.ErrNoAccounts):
			default:
				return nil, "", err
			}
		}
		if earliest != nil {
			return nil, "", earliest
		}
		return nil, "", pool.ErrNoAccounts
	}
	f := pool.Filter{Prefer: run.Steps[idx].Account}
	if step.Agent != "any" {
		f.Providers = []string{step.Agent}
	}
	l, err := e.Pool.Acquire(ctx, f)
	return l, "", err
}

// park records that the run is out of capacity and waits until the earliest reset.
func (e *Engine) park(ctx context.Context, run *model.Run, stepID string, until time.Time) error {
	run.Status = model.WaitingCapacity
	run.WaitUntil = until
	if err := e.save(run); err != nil {
		return err
	}
	e.emit(run.ID, stepID, "run.waiting_capacity", map[string]any{"until": until})
	if err := e.sleep(ctx, until.Sub(e.now())); err != nil {
		return err
	}
	run.Status = model.Running
	run.WaitUntil = time.Time{}
	e.emit(run.ID, stepID, "run.capacity_back", nil)
	return e.save(run)
}

// noteInterrupted commits the half-finished work and writes down what the next account needs to know.
func (e *Engine) noteInterrupted(ctx context.Context, run *model.Run, step workflow.Step, account pool.Account) {
	_, _ = e.Worktrees.CommitAll(ctx, run.Worktree, "legatus: "+step.ID+" (interrupted by a usage limit)")
	stat, _ := e.Worktrees.DiffStat(ctx, run.Worktree, run.BaseCommit)
	if strings.TrimSpace(stat) == "" {
		stat = "(no files changed yet)"
	}
	run.Handoff = fmt.Sprintf("An earlier login (%s) ran out of usage while working on this step. Everything it had written is already in this worktree and committed. Files changed so far compared with the start:\n%s", account.ID, strings.TrimSpace(stat))
	_ = e.save(run)
}

func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// runShell runs one command line in dir and returns the tail of its combined output.
func runShell(ctx context.Context, dir, command string, env []string, timeout time.Duration) (string, int, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := shellCommand(cctx, command)
	cmd.Dir = dir
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	text := out.String()
	if len(text) > outputTail {
		text = "…" + text[len(text)-outputTail:]
	}
	if err == nil {
		return text, 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return text, exit.ExitCode(), nil
	}
	if cctx.Err() == context.DeadlineExceeded {
		return text, -1, fmt.Errorf("timed out after %s", timeout)
	}
	return text, -1, err
}
