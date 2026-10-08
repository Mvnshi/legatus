// Package model holds the plain data types shared by every part of Legatus.
package model

import "time"

// Status is the state of a run or of one of its steps.
type Status string

const (
	Queued          Status = "queued"
	Running         Status = "running"
	WaitingCapacity Status = "waiting_capacity" // every eligible account is at its usage limit
	NeedsHuman      Status = "needs_human"      // a review or check wants a person to decide
	Succeeded       Status = "succeeded"
	Failed          Status = "failed"
	Canceled        Status = "canceled"
	Pending         Status = "pending" // a step that has not started
)

// Terminal reports whether nothing more will happen to a run in this status.
func (s Status) Terminal() bool {
	return s == Succeeded || s == Failed || s == Canceled
}

// StepKind is what a workflow step does.
type StepKind string

const (
	StepAgent  StepKind = "agent"
	StepCheck  StepKind = "check"
	StepReview StepKind = "review"
)

// Task is what the user asked for.
type Task struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Prompt     string    `json:"prompt"`
	Repo       string    `json:"repo"`
	Base       string    `json:"base"`
	Workflow   string    `json:"workflow"`
	Source     string    `json:"source,omitempty"`     // e.g. "github:owner/repo#12"
	Automation string    `json:"automation,omitempty"` // the automation that created this task, if any
	OpenPR     string    `json:"open_pr,omitempty"`    // "draft" or "ready": open a pull request when the run succeeds
	CreatedAt  time.Time `json:"created_at"`
}

// StepState is the progress of one workflow step inside a run.
type StepState struct {
	ID        string    `json:"id"`
	Kind      StepKind  `json:"kind"`
	Status    Status    `json:"status"`
	Attempts  int       `json:"attempts"`
	Account   string    `json:"account,omitempty"` // the last account that worked on it
	Provider  string    `json:"provider,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

// Run is one execution of a task through a workflow.
type Run struct {
	ID         string         `json:"id"`
	Task       Task           `json:"task"`
	Branch     string         `json:"branch"`
	Worktree   string         `json:"worktree"`
	BaseCommit string         `json:"base_commit,omitempty"` // the commit the branch started from; diffs are against it
	Status     Status         `json:"status"`
	Steps      []StepState    `json:"steps"`
	Current    int            `json:"current"`              // index of the step being worked on
	WaitUntil  time.Time      `json:"wait_until,omitempty"` // when a waiting_capacity run may try again
	Handoff    string         `json:"handoff,omitempty"`    // what the next attempt must know after an interruption
	Feedback   string         `json:"feedback,omitempty"`   // why the last check or review sent the run back
	Retries    map[string]int `json:"retries,omitempty"`    // how often each check or review has sent the run back
	Error      string         `json:"error,omitempty"`
	PRURL      string         `json:"pr_url,omitempty"` // the pull request opened from this run, if any
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Event is one line of a run's journal.
type Event struct {
	Time time.Time      `json:"time"`
	Run  string         `json:"run"`
	Step string         `json:"step,omitempty"`
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}
