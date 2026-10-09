// Package engine walks a run through its workflow: agent steps, checks and independent reviews. Its job
// that sets Legatus apart is what happens at a usage limit: the interrupted step continues on another
// login (or waits for the first one to reset) and the rest of the workflow still runs.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/github"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/pool"
	"github.com/Mvnshi/legatus/internal/sandbox"
	"github.com/Mvnshi/legatus/internal/store"
	"github.com/Mvnshi/legatus/internal/workflow"
	"github.com/Mvnshi/legatus/internal/worktree"
)

// PROpener pushes a branch and opens a pull request. *github.Client is the real one.
type PROpener interface {
	OpenPR(ctx context.Context, o github.PROptions) (string, error)
}

// Engine runs workflows. Fields other than the first four are optional.
type Engine struct {
	Store     *store.Store
	Pool      *pool.Pool
	Worktrees *worktree.Manager
	Backends  map[string]agent.Backend // by provider name
	DataDir   string                   // worktrees live under DataDir/worktrees

	RedactPII bool              // also remove email addresses from prompts
	OnEvent   func(model.Event) // called for every journal event, as it happens
	Sleep     func(context.Context, time.Duration) error
	Now       func() time.Time
	Environ   func() []string // the environment agents are derived from; os.Environ when nil

	AgentTimeout time.Duration // per attempt, when a step sets none; 30 minutes when zero
	CheckTimeout time.Duration // per command, when a step sets none; 10 minutes when zero

	// GitHub opens pull requests; the real gh-based client when nil.
	GitHub PROpener

	// CheckSlots is how many check commands may run at once across all runs; 1 when zero. Builds and test
	// suites are the heaviest thing Legatus starts, and several at once can use up a machine's memory.
	CheckSlots int
	gateOnce   sync.Once
	gate       chan struct{}
	// shell runs one check command; runShell unless a test replaces it.
	shell func(ctx context.Context, dir, command string, env []string, timeout time.Duration) (string, int, error)

	pfMu   sync.Mutex
	pfDone map[string]error // sandbox check results by account, for this process
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) sleep(ctx context.Context, d time.Duration) error {
	if e.Sleep != nil {
		return e.Sleep(ctx, d)
	}
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// acquireCheck waits for a free check slot and returns the function that gives it back. While it waits it
// says so in the run's journal, so a person looking at a "running" run can see why nothing is happening.
func (e *Engine) acquireCheck(ctx context.Context, runID, stepID string) (func(), error) {
	e.gateOnce.Do(func() {
		n := e.CheckSlots
		if n < 1 {
			n = 1
		}
		e.gate = make(chan struct{}, n)
	})
	select {
	case e.gate <- struct{}{}:
	default:
		e.emit(runID, stepID, "check.waiting", map[string]any{"reason": "another run is using the check slot"})
		select {
		case e.gate <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return func() { <-e.gate }, nil
}

func (e *Engine) environ() []string {
	if e.Environ != nil {
		return e.Environ()
	}
	return os.Environ()
}

func newID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// emit writes one journal event and passes it to OnEvent.
func (e *Engine) emit(runID, step, typ string, data map[string]any) {
	// Times are written the same way live and in the journal.
	for k, v := range data {
		if t, ok := v.(time.Time); ok {
			data[k] = t.UTC().Format(time.RFC3339)
		}
	}
	ev := model.Event{Time: e.now().UTC(), Run: runID, Step: step, Type: typ, Data: data}
	_ = e.Store.Append(ev)
	if e.OnEvent != nil {
		e.OnEvent(ev)
	}
}

func (e *Engine) save(run *model.Run) error { return e.Store.Save(run) }

// NewRun creates the run record and its worktree. It does not start the work.
func (e *Engine) NewRun(ctx context.Context, task model.Task, wf *workflow.Workflow) (*model.Run, error) {
	if err := wf.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(task.Prompt) == "" {
		return nil, errors.New("a task needs a prompt")
	}
	if task.Repo == "" {
		return nil, errors.New("a task needs a repository")
	}
	id := newID()
	task.ID = id
	task.Workflow = wf.Name
	if task.CreatedAt.IsZero() {
		task.CreatedAt = e.now().UTC()
	}
	// Secrets are removed before anything is stored: the run file, the journal and the evidence report
	// (which ends up in pull requests) only ever hold the redacted text.
	var findings []sandbox.Finding
	task.Prompt, findings = sandbox.Redact(task.Prompt, e.RedactPII)
	if task.Title == "" {
		task.Title = firstLine(task.Prompt, 80)
	} else {
		task.Title, _ = sandbox.Redact(task.Title, e.RedactPII)
	}
	branch := "legatus/" + id
	dir := filepath.Join(e.DataDir, "worktrees", id)
	// The base is pinned now, so the task means the same commit however long it waits in the queue. The
	// worktree itself is created when the run starts, so a long queue does not fill the disk.
	head, err := e.Worktrees.Resolve(ctx, task.Repo, task.Base)
	if err != nil {
		return nil, err
	}
	run := &model.Run{
		ID: id, Task: task, Branch: branch, Worktree: dir, BaseCommit: head,
		Status: model.Queued, CreatedAt: e.now().UTC(),
	}
	for _, s := range wf.Steps {
		run.Steps = append(run.Steps, model.StepState{ID: s.ID, Kind: s.Kind(), Status: model.Pending})
	}
	if err := e.save(run); err != nil {
		return nil, err
	}
	if err := e.saveWorkflow(id, wf); err != nil {
		return nil, err
	}
	e.emit(id, "", "run.created", map[string]any{"title": task.Title, "branch": branch, "base": head, "workflow": wf.Name, "redacted": sandbox.Summary(findings)})
	return run, nil
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > max {
		s = s[:max-1] + "…"
	}
	return s
}

type outcomeKind int

const (
	outOK outcomeKind = iota
	outRetry
	outHuman
	outFail
)

type outcome struct {
	kind     outcomeKind
	retryTo  int
	feedback string
	message  string
}

// Execute runs the workflow from the run's current step until the run succeeds, fails, needs a person,
// or ctx is cancelled. Cancelling leaves the run resumable: call Execute again to continue. It returns
// an error only when ctx ends or the run cannot be saved; a failed run is reported in its status.
func (e *Engine) Execute(ctx context.Context, id string, wf *workflow.Workflow) error {
	run, err := e.Store.Load(id)
	if err != nil {
		return err
	}
	if run.Status.Terminal() || run.Status == model.NeedsHuman {
		return nil
	}
	if len(run.Steps) != len(wf.Steps) {
		return fmt.Errorf("run %s was made with a different workflow (%d steps, now %d)", id, len(run.Steps), len(wf.Steps))
	}
	if run.Status == model.Running || run.Status == model.WaitingCapacity {
		e.emit(id, "", "run.resumed", map[string]any{"from_step": run.Current, "was": string(run.Status)})
	}
	run.Status = model.Running
	run.Error = ""
	run.WaitUntil = time.Time{}
	if err := e.save(run); err != nil {
		return err
	}
	if _, err := os.Stat(run.Worktree); errors.Is(err, os.ErrNotExist) {
		if err := e.Worktrees.Create(ctx, run.Task.Repo, run.BaseCommit, run.Branch, run.Worktree); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			run.Status = model.Failed
			run.Error = "could not create the worktree: " + err.Error()
			e.emit(id, "", "run.failed", map[string]any{"error": run.Error})
			return e.finish(ctx, run, wf)
		}
		e.emit(id, "", "worktree.created", map[string]any{"path": run.Worktree})
	}
	if run.Current == 0 && run.Steps[0].Attempts == 0 {
		e.emit(id, "", "run.started", nil)
	}

	for run.Current < len(wf.Steps) {
		if err := ctx.Err(); err != nil {
			return err
		}
		idx := run.Current
		step := wf.Steps[idx]
		st := &run.Steps[idx]
		if st.StartedAt.IsZero() {
			st.StartedAt = e.now().UTC()
		}
		st.Status = model.Running
		if err := e.save(run); err != nil {
			return err
		}
		e.emit(id, step.ID, "step.started", map[string]any{"type": step.Type, "index": idx})

		var out outcome
		switch step.Kind() {
		case model.StepAgent:
			out, err = e.agentStep(ctx, run, wf, idx)
		case model.StepCheck:
			out, err = e.checkStep(ctx, run, wf, idx)
		case model.StepReview:
			out, err = e.reviewStep(ctx, run, wf, idx)
		}
		if err != nil {
			if ctx.Err() != nil {
				_ = e.save(run)
				return ctx.Err()
			}
			out = outcome{kind: outFail, message: err.Error()}
		}

		switch out.kind {
		case outOK:
			st.Status = model.Succeeded
			st.EndedAt = e.now().UTC()
			e.emit(id, step.ID, "step.succeeded", map[string]any{"summary": st.Summary})
			run.Current++
		case outRetry:
			if run.Retries == nil {
				run.Retries = map[string]int{}
			}
			run.Retries[step.ID]++
			run.Feedback = out.feedback
			st.Status = model.Failed
			st.Error = out.message
			st.EndedAt = e.now().UTC()
			e.emit(id, step.ID, "step.sent_back", map[string]any{"to": wf.Steps[out.retryTo].ID, "reason": out.message, "times": run.Retries[step.ID]})
			for i := out.retryTo; i <= idx; i++ {
				run.Steps[i].Status = model.Pending
				run.Steps[i].EndedAt = time.Time{}
			}
			run.Current = out.retryTo
		case outHuman:
			st.Status = model.NeedsHuman
			st.Error = out.message
			st.EndedAt = e.now().UTC()
			run.Status = model.NeedsHuman
			run.Error = out.message
			e.emit(id, step.ID, "run.needs_human", map[string]any{"reason": out.message})
			return e.finish(ctx, run, wf)
		case outFail:
			st.Status = model.Failed
			st.Error = out.message
			st.EndedAt = e.now().UTC()
			run.Status = model.Failed
			run.Error = out.message
			e.emit(id, step.ID, "run.failed", map[string]any{"error": out.message})
			return e.finish(ctx, run, wf)
		}
		if err := e.save(run); err != nil {
			return err
		}
	}

	// Everything passed. Commit whatever is left, including files that checks wrote.
	if committed, err := e.Worktrees.CommitAll(ctx, run.Worktree, "legatus: finalize "+run.ID); err != nil {
		run.Status = model.Failed
		run.Error = "could not commit the result: " + err.Error()
		e.emit(id, "", "run.failed", map[string]any{"error": run.Error})
		return e.finish(ctx, run, wf)
	} else if committed {
		e.emit(id, "", "run.committed", map[string]any{"message": "finalize"})
	}
	run.Status = model.Succeeded
	e.emit(id, "", "run.succeeded", map[string]any{"branch": run.Branch})
	return e.finish(ctx, run, wf)
}

// finish writes the evidence report and then saves the final state. In that order, so a person watching
// never sees a finished run whose report is not there yet.
func (e *Engine) finish(ctx context.Context, run *model.Run, wf *workflow.Workflow) error {
	evidenceErr := e.writeEvidence(ctx, run, wf)
	if err := e.save(run); err != nil {
		return err
	}
	return evidenceErr
}

// saveWorkflow keeps the workflow with the run, so the run can be resumed without being told it again.
func (e *Engine) saveWorkflow(id string, wf *workflow.Workflow) error {
	data, err := wf.Marshal()
	if err != nil {
		return err
	}
	dir, err := e.Store.RunDir(id)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "workflow.yaml"), data, 0o600)
}

// LoadWorkflow reads the workflow a run was created with.
func (e *Engine) LoadWorkflow(id string) (*workflow.Workflow, error) {
	dir, err := e.Store.RunDir(id)
	if err != nil {
		return nil, err
	}
	return workflow.Load(filepath.Join(dir, "workflow.yaml"))
}

// Resume continues a run with the workflow it was created with.
func (e *Engine) Resume(ctx context.Context, id string) error {
	wf, err := e.LoadWorkflow(id)
	if err != nil {
		return err
	}
	return e.Execute(ctx, id, wf)
}

// MarkCanceled ends a run that has not finished because a person cancelled it. The branch and anything
// committed on it stay; the worktree is left for inspection (legatus clean removes it).
func (e *Engine) MarkCanceled(ctx context.Context, id string) error {
	run, err := e.Store.Load(id)
	if err != nil {
		return err
	}
	if run.Status.Terminal() {
		return nil
	}
	run.Status = model.Canceled
	run.Error = "canceled"
	run.WaitUntil = time.Time{}
	if run.Current < len(run.Steps) && run.Steps[run.Current].Status == model.Running {
		run.Steps[run.Current].Status = model.Canceled
		run.Steps[run.Current].EndedAt = e.now().UTC()
	}
	e.emit(id, "", "run.canceled", nil)
	wf, werr := e.LoadWorkflow(id)
	if werr != nil {
		return e.save(run)
	}
	return e.finish(ctx, run, wf)
}

// Requeue puts a failed, cancelled or person-needed run back in line to continue from the step that
// stopped it. Work already committed on its branch is kept.
func (e *Engine) Requeue(id string) error {
	run, err := e.Store.Load(id)
	if err != nil {
		return err
	}
	switch run.Status {
	case model.Failed, model.NeedsHuman, model.Canceled:
	default:
		return fmt.Errorf("run %s is %s; only a failed, cancelled or needs-a-person run can be retried", id, run.Status)
	}
	if run.Current >= len(run.Steps) {
		run.Current = len(run.Steps) - 1
	}
	for i := run.Current; i < len(run.Steps); i++ {
		run.Steps[i].Status = model.Pending
		run.Steps[i].Error = ""
		run.Steps[i].EndedAt = time.Time{}
	}
	run.Status = model.Queued
	run.Error = ""
	run.WaitUntil = time.Time{}
	run.Retries = nil
	if err := e.save(run); err != nil {
		return err
	}
	e.emit(id, run.Steps[run.Current].ID, "run.requeued", nil)
	return nil
}
