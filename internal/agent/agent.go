// Package agent is the seam between Legatus and the coding-agent programs it drives (Codex, Claude Code,
// and others). A Backend runs one prompt to completion inside a directory, as one login, and reports
// what happened as events.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/Mvnshi/legatus/internal/pool"
)

// Sandbox is what the agent may do. A backend turns it into the agent's own flags.
type Sandbox struct {
	ReadOnly bool // the agent may read but not change files (reviewers)
	Network  bool // the agent may use the network
}

// Request is one prompt to run.
type Request struct {
	Dir     string       // the run's worktree; the agent works here
	Prompt  string       // already redacted
	Account pool.Account // the login to use
	Env     []string     // the process environment, already scrubbed and holding the account's settings
	Sandbox Sandbox
	Timeout time.Duration // 0 means no limit beyond the context
	Role    string        // "implement" or "review"
}

// EventKind classifies what an agent reported.
type EventKind string

const (
	Message EventKind = "message" // the agent said something
	Tool    EventKind = "tool"    // the agent ran a tool or command
	Usage   EventKind = "usage"   // token or cost numbers
	Info    EventKind = "info"
)

// Event is one thing the agent reported while working.
type Event struct {
	Kind EventKind
	Text string
	Data map[string]any
}

// Result is how a run of the agent ended when it did not fail.
type Result struct {
	Summary string // the agent's final message
}

// LimitError means the login ran out of usage. ResetAt is when it comes back, if the agent said.
type LimitError struct {
	ResetAt time.Time
	Message string
}

func (e *LimitError) Error() string {
	if e.ResetAt.IsZero() {
		return "usage limit reached: " + e.Message
	}
	return fmt.Sprintf("usage limit reached until %s: %s", e.ResetAt.Format(time.RFC3339), e.Message)
}

// Backend drives one kind of agent.
type Backend interface {
	// Provider is the name accounts use to select this backend ("codex", "claude").
	Provider() string
	// AccountEnv is the environment this backend needs to run as the given login, such as the variable
	// that points at the login's configuration directory.
	AccountEnv(a pool.Account) map[string]string
	// Run does the work. It returns a *LimitError when the login is out of usage, and any other error for
	// a failure. It must stop promptly when ctx is cancelled.
	Run(ctx context.Context, req Request, emit func(Event)) (Result, error)
}
