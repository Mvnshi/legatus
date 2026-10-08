package cli

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/fake"
	"github.com/Mvnshi/legatus/internal/app"
	"github.com/Mvnshi/legatus/internal/pool"
)

func pause(ctx context.Context, d time.Duration) error {
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

// demoApp is an installation whose agents are stand-ins: the first Codex login runs out of usage the first
// time it is used, the others finish the work, and a Claude login reviews it. delay slows each agent step
// so that progress can be watched.
func demoApp(root string, delay time.Duration) (*app.App, error) {
	author := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		runName := filepath.Base(req.Dir)
		if err := pause(ctx, delay); err != nil {
			return agent.Result{}, err
		}
		if req.Account.ID == "work-1" {
			emit(agent.Event{Kind: agent.Tool, Text: "write half-done-" + runName + ".txt"})
			_ = os.WriteFile(filepath.Join(req.Dir, "half-done-"+runName+".txt"), []byte("the first login got this far\n"), 0o600)
			return agent.Result{}, &agent.LimitError{ResetAt: time.Now().Add(3 * time.Hour), Message: "You've hit your usage limit."}
		}
		emit(agent.Event{Kind: agent.Tool, Text: "write feature-" + runName + ".txt"})
		if err := pause(ctx, delay); err != nil {
			return agent.Result{}, err
		}
		_ = os.WriteFile(filepath.Join(req.Dir, "feature-"+runName+".txt"), []byte("finished by "+req.Account.ID+"\n"), 0o600)
		emit(agent.Event{Kind: agent.Message, Text: "Finished the task."})
		return agent.Result{Summary: "added feature-" + runName + ".txt"}, nil
	})
	reviewer := fake.New("claude", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if err := pause(ctx, delay); err != nil {
			return agent.Result{}, err
		}
		return agent.Result{Summary: "The change matches the task.\n" + `{"verdict":"approve","summary":"matches the task","issues":[]}`}, nil
	})
	a, err := app.Open(filepath.Join(root, "legatus"), app.Options{Backends: map[string]agent.Backend{"codex": author, "claude": reviewer}})
	if err != nil {
		return nil, err
	}
	for _, acct := range []pool.Account{
		{ID: "work-1", Provider: "codex", MaxConcurrent: 2},
		{ID: "work-2", Provider: "codex", MaxConcurrent: 2},
		{ID: "second-opinion", Provider: "claude", MaxConcurrent: 2},
	} {
		if err := a.Pool.Add(acct); err != nil {
			return nil, err
		}
	}
	a.Pool.SetUsage("work-1", 100) // the first login is the first choice, so the demo is repeatable
	a.Pool.SetUsage("work-2", 50)
	return a, nil
}
