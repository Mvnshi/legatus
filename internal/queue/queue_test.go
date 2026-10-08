package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/fake"
	"github.com/Mvnshi/legatus/internal/engine"
	"github.com/Mvnshi/legatus/internal/hub"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/pool"
	"github.com/Mvnshi/legatus/internal/store"
	"github.com/Mvnshi/legatus/internal/workflow"
	"github.com/Mvnshi/legatus/internal/worktree"
)

type rig struct {
	t     *testing.T
	repo  string
	eng   *engine.Engine
	store *store.Store
	hub   *hub.Hub
	be    *fake.Backend
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newRig(t *testing.T, h fake.Handler) *rig {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "config", "user.name", "Tester")
	git(t, repo, "config", "user.email", "tester@example.com")
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o600)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "first")

	data := t.TempDir()
	st, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	p := pool.New()
	for _, id := range []string{"a", "b", "c"} {
		p.Add(pool.Account{ID: id, Provider: "codex", MaxConcurrent: 10})
	}
	be := fake.New("codex", h)
	eng := &engine.Engine{
		Store: st, Pool: p, Worktrees: worktree.New(), Backends: map[string]agent.Backend{"codex": be}, DataDir: data,
	}
	return &rig{t: t, repo: repo, eng: eng, store: st, hub: hub.New(), be: be}
}

func (r *rig) scheduler(concurrency int) *Scheduler {
	return &Scheduler{Engine: r.eng, Hub: r.hub, Concurrency: concurrency}
}

func (r *rig) task(prompt string) model.Task {
	return model.Task{Prompt: prompt, Repo: r.repo, Base: "main"}
}

func (r *rig) wait(id string, want ...model.Status) *model.Run {
	r.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		run, err := r.store.Load(id)
		if err == nil {
			for _, w := range want {
				if run.Status == w {
					return run
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	run, _ := r.store.Load(id)
	r.t.Fatalf("run %s never reached %v; it is %+v", id, want, run)
	return nil
}

func defaultWF() *workflow.Workflow { return workflow.Default([]string{"git --version"}, false) }

func TestManyRunsFinishAndNoMoreThanTheLimitWorkAtOnce(t *testing.T) {
	var now, peak int32
	r := newRig(t, func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		n := atomic.AddInt32(&now, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(600 * time.Millisecond)
		atomic.AddInt32(&now, -1)
		return fake.WriteFile(ctx, req, call, emit)
	})
	s := r.scheduler(2)
	var ids []string
	for i := 0; i < 8; i++ {
		run, err := s.Submit(context.Background(), r.task(fmt.Sprintf("task %d", i)), defaultWF())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, run.ID)
	}
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	for _, id := range ids {
		if run := r.wait(id, model.Succeeded, model.Failed); run.Status != model.Succeeded {
			t.Fatalf("run %s: %s %s", id, run.Status, run.Error)
		}
	}
	if peak > 2 {
		t.Fatalf("%d runs worked at once with a limit of 2", peak)
	}
	if peak < 2 {
		t.Fatalf("the workers did not run in parallel (peak %d)", peak)
	}
}

func TestAWorktreeIsCreatedWhenARunStartsNotWhenItIsQueued(t *testing.T) {
	r := newRig(t, nil)
	s := r.scheduler(1)
	run, err := s.Submit(context.Background(), r.task("task"), defaultWF())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(run.Worktree); !os.IsNotExist(err) {
		t.Fatalf("a queued run already has a worktree: %v", err)
	}
	if run.BaseCommit == "" {
		t.Fatal("the base commit should be pinned when the task is submitted")
	}
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	done := r.wait(run.ID, model.Succeeded, model.Failed)
	if done.Status != model.Succeeded {
		t.Fatalf("%s: %s", done.Status, done.Error)
	}
	if _, err := os.Stat(done.Worktree); err != nil {
		t.Fatalf("no worktree after the run: %v", err)
	}
}

func TestCancellingAQueuedRunMeansItNeverStarts(t *testing.T) {
	r := newRig(t, nil)
	s := r.scheduler(1)
	run, _ := s.Submit(context.Background(), r.task("task"), defaultWF())
	if err := s.Cancel(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	got := r.wait(run.ID, model.Canceled)
	time.Sleep(100 * time.Millisecond)
	if len(r.be.Calls()) != 0 || got.Status != model.Canceled {
		t.Fatalf("a cancelled queued run did work: %d calls, %s", len(r.be.Calls()), got.Status)
	}
}

func TestCancellingAWorkingRunStopsItAndFreesTheAccount(t *testing.T) {
	started := make(chan struct{}, 1)
	r := newRig(t, func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		started <- struct{}{}
		<-ctx.Done()
		return agent.Result{}, ctx.Err()
	})
	s := r.scheduler(1)
	defer s.Stop()
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	run, _ := s.Submit(context.Background(), r.task("task"), defaultWF())
	<-started
	if err := s.Cancel(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	got := r.wait(run.ID, model.Canceled)
	if got.Error != "canceled" {
		t.Fatalf("error = %q", got.Error)
	}
	for _, a := range r.eng.Pool.Snapshots() {
		if a.Active != 0 {
			t.Fatalf("account %s is still leased after a cancel", a.ID)
		}
	}
	if err := s.Cancel(context.Background(), run.ID); err == nil {
		t.Fatal("cancelling a finished run should say so")
	}
}

func TestUnfinishedRunsResumeWhenTheProgramStartsAgain(t *testing.T) {
	r := newRig(t, nil)
	wf := defaultWF()
	queued, _ := r.eng.NewRun(context.Background(), r.task("was waiting"), wf)
	interrupted, _ := r.eng.NewRun(context.Background(), r.task("was working"), wf)
	interrupted.Status = model.Running
	if err := r.store.Save(interrupted); err != nil {
		t.Fatal(err)
	}
	finished, _ := r.eng.NewRun(context.Background(), r.task("already done"), wf)
	finished.Status = model.Succeeded
	r.store.Save(finished)
	needsPerson, _ := r.eng.NewRun(context.Background(), r.task("waiting for a person"), wf)
	needsPerson.Status = model.NeedsHuman
	r.store.Save(needsPerson)

	s := r.scheduler(2)
	resumed, err := s.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if resumed != 2 {
		t.Fatalf("resumed %d runs, want 2 (the queued and the interrupted one)", resumed)
	}
	for _, id := range []string{queued.ID, interrupted.ID} {
		if run := r.wait(id, model.Succeeded, model.Failed); run.Status != model.Succeeded {
			t.Fatalf("%s: %s %s", id, run.Status, run.Error)
		}
	}
	if got, _ := r.store.Load(needsPerson.ID); got.Status != model.NeedsHuman {
		t.Fatalf("a run that needs a person was touched: %s", got.Status)
	}
}

func TestStoppingLeavesWorkingRunsResumable(t *testing.T) {
	started := make(chan struct{}, 1)
	var first sync.Once
	r := newRig(t, func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		blocked := false
		first.Do(func() { blocked = true })
		if blocked {
			started <- struct{}{}
			<-ctx.Done()
			return agent.Result{}, ctx.Err()
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	s := r.scheduler(1)
	s.Start(context.Background())
	run, _ := s.Submit(context.Background(), r.task("task"), defaultWF())
	<-started
	s.Stop()
	mid, _ := r.store.Load(run.ID)
	if mid.Status != model.Running {
		t.Fatalf("after Stop the run is %s, want running (resumable)", mid.Status)
	}
	s2 := r.scheduler(1)
	if n, err := s2.Start(context.Background()); err != nil || n != 1 {
		t.Fatalf("restart resumed %d (%v)", n, err)
	}
	defer s2.Stop()
	if done := r.wait(run.ID, model.Succeeded, model.Failed); done.Status != model.Succeeded {
		t.Fatalf("%s: %s", done.Status, done.Error)
	}
}

func TestRetryContinuesAFailedRun(t *testing.T) {
	var broken atomic.Bool
	broken.Store(true)
	r := newRig(t, func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if broken.Load() {
			return agent.Result{}, errors.New("the agent crashed")
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	s := r.scheduler(1)
	s.Start(context.Background())
	defer s.Stop()
	run, _ := s.Submit(context.Background(), r.task("task"), defaultWF())
	failed := r.wait(run.ID, model.Failed, model.Succeeded)
	if failed.Status != model.Failed {
		t.Fatalf("status %s", failed.Status)
	}
	broken.Store(false)
	if err := s.Retry(run.ID); err != nil {
		t.Fatal(err)
	}
	if done := r.wait(run.ID, model.Succeeded); done.Status != model.Succeeded {
		t.Fatalf("%s: %s", done.Status, done.Error)
	}
	if err := s.Retry(run.ID); err == nil {
		t.Fatal("retrying a succeeded run should be refused")
	}
}

func TestTheHubIsToldAboutChanges(t *testing.T) {
	r := newRig(t, nil)
	s := r.scheduler(1)
	s.Start(context.Background())
	defer s.Stop()
	ch := r.hub.Changed()
	run, _ := s.Submit(context.Background(), r.task("task"), defaultWF())
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("the hub was not notified of a new run")
	}
	r.wait(run.ID, model.Succeeded)
	if st := s.Stats(); st.Queued != 0 {
		t.Fatalf("stats = %+v", st)
	}
}
