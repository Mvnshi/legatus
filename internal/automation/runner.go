package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Mvnshi/legatus/internal/github"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/store"
	"github.com/Mvnshi/legatus/internal/workflow"
)

// Submitter starts runs; *queue.Scheduler is the real one.
type Submitter interface {
	Submit(ctx context.Context, task model.Task, wf *workflow.Workflow) (*model.Run, error)
}

// RunLister reads the existing runs; *store.Store is the real one.
type RunLister interface {
	List() ([]*model.Run, error)
}

// GitHubAPI is the part of the GitHub client the watcher uses; *github.Client is the real one.
type GitHubAPI interface {
	ListIssues(ctx context.Context, slug, label string, limit int) ([]github.IssueSummary, error)
	Issue(ctx context.Context, ref, dir string) (*github.Issue, error)
}

// Status is what the cockpit shows for one automation.
type Status struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"` // "schedule" or "github"
	When     string    `json:"when"` // the schedule, or what is watched
	Disabled bool      `json:"disabled"`
	Last     time.Time `json:"last,omitempty"`
	Next     time.Time `json:"next,omitempty"`
	Active   int       `json:"active"`
	Note     string    `json:"note,omitempty"` // the latest thing worth saying: skipped, failed, started
}

type state struct {
	Last   map[string]time.Time         `json:"last"`   // when each automation last started a run (or was first seen)
	Polled map[string]time.Time         `json:"polled"` // when each watch last looked
	Seen   map[string]map[string]string `json:"seen"`   // issue number -> the run that took it, per automation
	Ran    map[string]time.Time         `json:"ran"`    // when each automation last actually started a run
}

// Runner decides what is due and submits it. It is safe for concurrent use.
type Runner struct {
	Dir       string // holds automations.yaml and automation-state.json
	Submitter Submitter
	Runs      RunLister
	GitHub    GitHubAPI
	Now       func() time.Time
	Log       func(format string, args ...any)

	mu     sync.Mutex
	cfg    *Config
	cfgMod time.Time
	cfgErr error
	st     state
	loaded bool
	notes  map[string]string
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) logf(format string, args ...any) {
	if r.Log != nil {
		r.Log(format, args...)
	}
}

func (r *Runner) configPath() string { return filepath.Join(r.Dir, "automations.yaml") }
func (r *Runner) statePath() string  { return filepath.Join(r.Dir, "automation-state.json") }

// reload picks up edits to the file. A file that no longer parses leaves the last good configuration in
// place and the error is shown, so a typo does not silently stop everything.
func (r *Runner) reload() {
	if !r.loaded {
		r.loaded = true
		r.notes = map[string]string{}
		r.st = state{Last: map[string]time.Time{}, Polled: map[string]time.Time{}, Seen: map[string]map[string]string{}, Ran: map[string]time.Time{}}
		if data, err := os.ReadFile(r.statePath()); err == nil {
			var s state
			if json.Unmarshal(data, &s) == nil {
				r.st = s
			}
		}
		if r.st.Last == nil {
			r.st.Last = map[string]time.Time{}
		}
		if r.st.Polled == nil {
			r.st.Polled = map[string]time.Time{}
		}
		if r.st.Seen == nil {
			r.st.Seen = map[string]map[string]string{}
		}
		if r.st.Ran == nil {
			r.st.Ran = map[string]time.Time{}
		}
	}
	info, err := os.Stat(r.configPath())
	switch {
	case errors.Is(err, os.ErrNotExist):
		r.cfg, r.cfgErr, r.cfgMod = &Config{}, nil, time.Time{}
		return
	case err != nil:
		r.cfgErr = err
		return
	}
	if r.cfg != nil && info.ModTime().Equal(r.cfgMod) {
		return
	}
	cfg, err := LoadConfig(r.configPath())
	r.cfgMod = info.ModTime()
	if err != nil {
		r.cfgErr = err
		if r.cfg == nil {
			r.cfg = &Config{}
		}
		return
	}
	r.cfg, r.cfgErr = cfg, nil
}

func (r *Runner) save() {
	data, err := json.MarshalIndent(r.st, "", "  ")
	if err != nil {
		return
	}
	tmp := r.statePath() + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = store.RenameReplace(tmp, r.statePath())
	}
}

func (r *Runner) activeRuns(id string) int {
	runs, err := r.Runs.List()
	if err != nil {
		return 0
	}
	n := 0
	for _, run := range runs {
		if run.Task.Automation != id {
			continue
		}
		switch run.Status {
		case model.Queued, model.Running, model.WaitingCapacity:
			n++
		}
	}
	return n
}

func workflowFor(a Automation) *workflow.Workflow {
	wf := workflow.Default(a.Checks, a.Review)
	agent := a.Agent
	if agent == "" {
		agent = "any"
	}
	wf.Steps[0].Agent = agent
	if a.NoSandbox {
		for i := range wf.Steps {
			if wf.Steps[i].Kind() != model.StepCheck {
				wf.Steps[i].Sandbox.Mode = "none"
			}
		}
	}
	return wf
}

func (r *Runner) submit(ctx context.Context, a Automation, t model.Task) (*model.Run, error) {
	t.Repo, t.Base, t.Automation, t.OpenPR = a.Repo, a.Base, a.ID, a.PR
	run, err := r.Submitter.Submit(ctx, t, workflowFor(a))
	if err == nil {
		r.st.Ran[a.ID] = r.now()
	}
	return run, err
}

// Tick does one pass: for every enabled automation, start whatever is due. It returns the runs it started.
func (r *Runner) Tick(ctx context.Context) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reload()
	now := r.now()
	var started []string
	for _, a := range r.cfg.Automations {
		if a.Disabled || ctx.Err() != nil {
			continue
		}
		if a.Schedule != "" {
			started = append(started, r.tickSchedule(ctx, a, now)...)
		} else if a.GitHub != nil {
			started = append(started, r.tickWatch(ctx, a, now)...)
		}
	}
	r.save()
	return started
}

func (r *Runner) tickSchedule(ctx context.Context, a Automation, now time.Time) []string {
	sched, err := ParseSchedule(a.Schedule)
	if err != nil {
		r.notes[a.ID] = err.Error()
		return nil
	}
	last, seen := r.st.Last[a.ID]
	if !seen {
		// First time this automation is seen: it counts from now. Enabling "daily 09:00" at 15:00 does not
		// start a run immediately; the first one is tomorrow at 09:00.
		r.st.Last[a.ID] = now
		return nil
	}
	if !sched.Due(last, now) {
		return nil
	}
	if r.activeRuns(a.ID) >= a.maxActive() {
		r.notes[a.ID] = "skipped: an earlier run of this automation is still going"
		return nil
	}
	run, err := r.submit(ctx, a, model.Task{Prompt: a.Prompt, Title: firstLine(a.Prompt)})
	if err != nil {
		r.notes[a.ID] = "could not start: " + err.Error()
		r.logf("automation %s: %v", a.ID, err)
		r.st.Last[a.ID] = now // do not retry every tick; the next scheduled time tries again
		return nil
	}
	r.st.Last[a.ID] = now
	r.notes[a.ID] = "started run " + run.ID
	return []string{run.ID}
}

func (r *Runner) tickWatch(ctx context.Context, a Automation, now time.Time) []string {
	w := a.GitHub
	if last, ok := r.st.Polled[a.ID]; ok && now.Sub(last) < w.WatchEvery() {
		return nil
	}
	r.st.Polled[a.ID] = now
	issues, err := r.GitHub.ListIssues(ctx, w.Repo, w.Label, 30)
	if err != nil {
		r.notes[a.ID] = "could not look at GitHub: " + err.Error()
		r.logf("automation %s: %v", a.ID, err)
		return nil
	}
	allowed := map[string]bool{}
	for _, au := range w.Authors {
		allowed[strings.ToLower(au)] = true
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Number < issues[j].Number }) // oldest first
	maxNew := w.MaxNew
	if maxNew <= 0 {
		maxNew = 2
	}
	if r.st.Seen[a.ID] == nil {
		r.st.Seen[a.ID] = map[string]string{}
	}
	var started []string
	var skipped []string
	for _, is := range issues {
		key := fmt.Sprint(is.Number)
		if _, done := r.st.Seen[a.ID][key]; done {
			continue
		}
		if !w.Anyone && !allowed[strings.ToLower(is.Author)] {
			skipped = append(skipped, fmt.Sprintf("#%d by %s", is.Number, is.Author))
			continue
		}
		if len(started) >= maxNew || r.activeRuns(a.ID)+len(started) >= a.maxActive() {
			break
		}
		issue, err := r.GitHub.Issue(ctx, fmt.Sprintf("%s#%d", w.Repo, is.Number), a.Repo)
		if err != nil {
			r.notes[a.ID] = fmt.Sprintf("could not read #%d: %v", is.Number, err)
			continue
		}
		if !w.Anyone && !allowed[strings.ToLower(issue.Author)] {
			continue // the author changed between the list and the read; do not run it
		}
		run, err := r.submit(ctx, a, model.Task{
			Prompt: github.TaskPrompt(issue), Source: "github:" + issue.Ref.String(),
			Title: fmt.Sprintf("Issue #%d: %s", issue.Ref.Number, issue.Title),
		})
		if err != nil {
			r.notes[a.ID] = fmt.Sprintf("could not start #%d: %v", is.Number, err)
			continue
		}
		r.st.Seen[a.ID][key] = run.ID
		started = append(started, run.ID)
	}
	switch {
	case len(started) > 0:
		r.notes[a.ID] = fmt.Sprintf("started %d run(s) from issues", len(started))
	case len(skipped) > 0:
		r.notes[a.ID] = "ignored " + strings.Join(skipped, ", ") + ": not from an author you listed"
	default:
		delete(r.notes, a.ID)
	}
	return started
}

// RunNow starts a scheduled automation immediately, whether or not it is due.
func (r *Runner) RunNow(ctx context.Context, id string) (*model.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reload()
	for _, a := range r.cfg.Automations {
		if a.ID != id {
			continue
		}
		if a.Schedule == "" {
			return nil, errors.New("only scheduled automations can be run by hand; a watch runs when it finds an issue")
		}
		if r.activeRuns(id) >= a.maxActive() {
			return nil, errors.New("an earlier run of this automation is still going")
		}
		run, err := r.submit(ctx, a, model.Task{Prompt: a.Prompt, Title: firstLine(a.Prompt)})
		if err != nil {
			return nil, err
		}
		r.st.Last[id] = r.now()
		r.notes[id] = "started run " + run.ID + " by hand"
		r.save()
		return run, nil
	}
	return nil, fmt.Errorf("no automation named %q", id)
}

// Status lists every automation with its state, and the configuration error if the file is broken.
func (r *Runner) Status() ([]Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reload()
	now := r.now()
	var out []Status
	for _, a := range r.cfg.Automations {
		s := Status{ID: a.ID, Disabled: a.Disabled, Active: r.activeRuns(a.ID), Note: r.notes[a.ID], Last: r.st.Ran[a.ID]}
		if a.Schedule != "" {
			s.Kind, s.When = "schedule", a.Schedule
			if sched, err := ParseSchedule(a.Schedule); err == nil && !a.Disabled {
				base := now
				if l, ok := r.st.Last[a.ID]; ok {
					base = l
				}
				s.Next = sched.Next(base)
				if sched.Due(base, now) {
					s.Next = now
				}
			}
		} else if a.GitHub != nil {
			s.Kind = "github"
			s.When = fmt.Sprintf("issues labelled %q in %s, every %s", a.GitHub.Label, a.GitHub.Repo, a.GitHub.WatchEvery())
			if p, ok := r.st.Polled[a.ID]; ok {
				if !a.Disabled {
					s.Next = p.Add(a.GitHub.WatchEvery())
				}
			}
		}
		out = append(out, s)
	}
	return out, r.cfgErr
}

// Run ticks until ctx ends.
func (r *Runner) Run(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 30 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	r.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.Tick(ctx)
		}
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 80 {
		s = s[:79] + "…"
	}
	return s
}
