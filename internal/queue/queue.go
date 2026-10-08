// Package queue runs many runs at once. It owns the line of waiting runs and a fixed number of workers,
// resumes whatever was unfinished when the program last stopped, and lets a person cancel or retry a run.
package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Mvnshi/legatus/internal/engine"
	"github.com/Mvnshi/legatus/internal/hub"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/workflow"
)

// DefaultConcurrency is how many runs work at once when none is set. The real limit on agent work is the
// account pool (each login takes its own number of tasks); this one bounds worktrees and check processes.
const DefaultConcurrency = 6

// Stats is a snapshot of the queue.
type Stats struct {
	Queued int `json:"queued"` // waiting for a worker
	Active int `json:"active"` // on a worker (including runs waiting for an account to reset)
}

// Scheduler is safe for concurrent use.
type Scheduler struct {
	Engine      *engine.Engine
	Hub         *hub.Hub // told about every journal event and queue change; may be nil
	Concurrency int

	mu       sync.Mutex
	pending  []string
	wake     chan struct{}
	active   map[string]context.CancelFunc
	canceled map[string]bool
	again    map[string]bool // asked to run again while still finishing (for example a retry that raced the end)
	ctx      context.Context
	stop     context.CancelFunc
	wg       sync.WaitGroup
}

// Start resumes unfinished runs (oldest first) and starts the workers. It returns how many it resumed.
// The workers stop when ctx ends or Stop is called; runs they were in the middle of stay resumable.
func (s *Scheduler) Start(ctx context.Context) (int, error) {
	s.mu.Lock()
	if s.ctx != nil {
		s.mu.Unlock()
		return 0, errors.New("the scheduler is already running")
	}
	s.ctx, s.stop = context.WithCancel(ctx)
	s.ensureLocked()
	s.mu.Unlock()

	if s.Hub != nil {
		prev := s.Engine.OnEvent
		s.Engine.OnEvent = func(ev model.Event) {
			if prev != nil {
				prev(ev)
			}
			s.Hub.Notify(ev.Run)
		}
	}

	runs, err := s.Engine.Store.List()
	if err != nil {
		return 0, err
	}
	resumed := 0
	for i := len(runs) - 1; i >= 0; i-- { // List is newest first
		switch runs[i].Status {
		case model.Queued, model.Running, model.WaitingCapacity:
			s.enqueue(runs[i].ID)
			resumed++
		}
	}
	n := s.Concurrency
	if n <= 0 {
		n = DefaultConcurrency
	}
	for i := 0; i < n; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return resumed, nil
}

// ensureLocked makes the internal maps and channel; the caller holds s.mu. Runs can be submitted before
// Start (the API may be up before the workers), so this does not wait for Start.
func (s *Scheduler) ensureLocked() {
	if s.wake == nil {
		s.wake = make(chan struct{})
	}
	if s.active == nil {
		s.active = map[string]context.CancelFunc{}
	}
	if s.canceled == nil {
		s.canceled = map[string]bool{}
	}
	if s.again == nil {
		s.again = map[string]bool{}
	}
}

// Stop stops the workers and waits for them. Runs in progress are left resumable.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	stop := s.stop
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
	s.wg.Wait()
}

// Submit creates a run and puts it in line.
func (s *Scheduler) Submit(ctx context.Context, task model.Task, wf *workflow.Workflow) (*model.Run, error) {
	run, err := s.Engine.NewRun(ctx, task, wf)
	if err != nil {
		return nil, err
	}
	s.enqueue(run.ID)
	return run, nil
}

// Retry puts a failed, cancelled or person-needed run back in line.
func (s *Scheduler) Retry(id string) error {
	if err := s.Engine.Requeue(id); err != nil {
		return err
	}
	s.enqueue(id)
	return nil
}

// Cancel stops a run: one waiting in line is removed at once, one working is told to stop.
func (s *Scheduler) Cancel(ctx context.Context, id string) error {
	s.mu.Lock()
	s.ensureLocked()
	for i, p := range s.pending {
		if p == id {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			s.mu.Unlock()
			return s.Engine.MarkCanceled(ctx, id)
		}
	}
	if cancel, ok := s.active[id]; ok {
		s.canceled[id] = true
		s.mu.Unlock()
		cancel()
		return nil
	}
	s.mu.Unlock()
	run, err := s.Engine.Store.Load(id)
	if err != nil {
		return err
	}
	if run.Status.Terminal() || run.Status == model.NeedsHuman {
		return fmt.Errorf("run %s is already %s", id, run.Status)
	}
	return s.Engine.MarkCanceled(ctx, id) // not queued here (for example, left over from an earlier process)
}

// Stats reports how many runs are waiting and working.
func (s *Scheduler) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{Queued: len(s.pending), Active: len(s.active)}
}

func (s *Scheduler) enqueue(id string) {
	s.mu.Lock()
	s.ensureLocked()
	for _, p := range s.pending {
		if p == id {
			s.mu.Unlock()
			return
		}
	}
	if _, ok := s.active[id]; ok {
		// Still finishing on a worker: run it again as soon as that worker is done with it.
		s.again[id] = true
		s.mu.Unlock()
		return
	}
	s.pending = append(s.pending, id)
	close(s.wake)
	s.wake = make(chan struct{})
	s.mu.Unlock()
	if s.Hub != nil {
		s.Hub.Notify(id)
	}
}

// next blocks until a run is waiting, or the scheduler stops.
func (s *Scheduler) next() (string, bool) {
	for {
		s.mu.Lock()
		if len(s.pending) > 0 {
			id := s.pending[0]
			s.pending = s.pending[1:]
			s.mu.Unlock()
			return id, true
		}
		wake := s.wake
		s.mu.Unlock()
		select {
		case <-s.ctx.Done():
			return "", false
		case <-wake:
		}
	}
}

func (s *Scheduler) worker() {
	defer s.wg.Done()
	for {
		id, ok := s.next()
		if !ok {
			return
		}
		s.runOne(id)
	}
}

func (s *Scheduler) runOne(id string) {
	ctx, cancel := context.WithCancel(s.ctx)
	s.mu.Lock()
	s.active[id] = cancel
	s.mu.Unlock()

	err := s.Engine.Resume(ctx, id)

	s.mu.Lock()
	delete(s.active, id)
	userCanceled := s.canceled[id]
	delete(s.canceled, id)
	runAgain := s.again[id]
	delete(s.again, id)
	s.mu.Unlock()
	cancel()

	switch {
	case userCanceled:
		_ = s.Engine.MarkCanceled(context.Background(), id)
	case s.ctx.Err() != nil:
		// The program is stopping. The run is left as it is and resumes on the next start.
	case err != nil:
		s.failRun(id, err)
	}
	if runAgain && s.ctx.Err() == nil && !userCanceled {
		s.enqueue(id)
	}
	if s.Hub != nil {
		s.Hub.Notify(id)
	}
}

// failRun records an error the engine could not handle itself (for example the disk was full).
func (s *Scheduler) failRun(id string, cause error) {
	run, err := s.Engine.Store.Load(id)
	if err != nil || run.Status.Terminal() {
		return
	}
	run.Status = model.Failed
	run.Error = "internal error: " + cause.Error()
	_ = s.Engine.Store.Save(run)
	_ = s.Engine.Store.Append(model.Event{Run: id, Type: "run.failed", Data: map[string]any{"error": run.Error}})
}
