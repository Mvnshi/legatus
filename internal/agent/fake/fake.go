// Package fake is a scriptable agent used by tests and by `legatus demo`. It does real file work in the
// worktree so the rest of the system (commits, diffs, checks) is exercised for real.
package fake

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/pool"
)

// Handler decides what one call does. call counts the calls made as this account, starting at 1.
type Handler func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error)

// Backend is an agent whose behaviour is a Handler.
type Backend struct {
	Name    string
	Handler Handler

	mu           sync.Mutex
	calls        map[string]int
	Log          []Call // every request received, in order
	PreflightErr error  // returned by Preflight
	Preflights   int    // how many times Preflight was called
}

// Call records one request.
type Call struct {
	Account string
	Role    string
	Prompt  string
	Env     []string
	Sandbox agent.Sandbox
}

// Preflight makes the fake behave like a backend whose sandbox check can fail: set PreflightErr.
func (b *Backend) Preflight(ctx context.Context, a pool.Account, dir string, env []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Preflights++
	return b.PreflightErr
}

// New returns a backend for the provider name whose behaviour is h. A nil h writes one file and succeeds.
func New(provider string, h Handler) *Backend {
	if h == nil {
		h = WriteFile
	}
	return &Backend{Name: provider, Handler: h, calls: map[string]int{}}
}

func (b *Backend) Provider() string { return b.Name }

func (b *Backend) AccountEnv(a pool.Account) map[string]string {
	return map[string]string{"FAKE_AGENT_HOME": a.Home}
}

func (b *Backend) Run(ctx context.Context, req agent.Request, emit func(agent.Event)) (agent.Result, error) {
	b.mu.Lock()
	b.calls[req.Account.ID]++
	call := b.calls[req.Account.ID]
	b.Log = append(b.Log, Call{Account: req.Account.ID, Role: req.Role, Prompt: req.Prompt, Env: req.Env, Sandbox: req.Sandbox})
	b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return agent.Result{}, err
	}
	return b.Handler(ctx, req, call, emit)
}

// Calls returns a copy of the request log.
func (b *Backend) Calls() []Call {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Call(nil), b.Log...)
}

// WriteFile writes notes-<account>-<call>.txt in the worktree and succeeds.
func WriteFile(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
	name := fmt.Sprintf("notes-%s-%d.txt", req.Account.ID, call)
	emit(agent.Event{Kind: agent.Tool, Text: "write " + name})
	if err := os.WriteFile(filepath.Join(req.Dir, name), []byte("work by "+req.Account.ID+"\n"), 0o600); err != nil {
		return agent.Result{}, err
	}
	emit(agent.Event{Kind: agent.Message, Text: "done"})
	return agent.Result{Summary: "wrote " + name}, nil
}
