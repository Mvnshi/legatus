// Package workflow reads and checks the YAML that says what a run does, step by step.
//
//	name: default
//	steps:
//	  - id: implement
//	    type: agent
//	    agent: any
//	  - id: checks
//	    type: check
//	    run: ["go test ./..."]
//	    on_fail: {retry_with: implement, max: 2}
//	  - id: review
//	    type: review
//	    agent: other-than:implement
//	    on_fail: {retry_with: implement, max: 1}
package workflow

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Mvnshi/legatus/internal/model"
)

// Duration reads values such as "90s" or "10m" from YAML.
type Duration time.Duration

func (d Duration) IsZero() bool { return d == 0 }

func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q", s)
	}
	*d = Duration(v)
	return nil
}

// OnFail says what to do when a check or review does not pass.
type OnFail struct {
	RetryWith string `yaml:"retry_with"` // id of an earlier agent step to run again with the failure as context
	Max       int    `yaml:"max"`        // how many times to go back; 0 means the run fails
}

// Sandbox is what an agent step may do. The zero value is the safe default: write inside the run's
// worktree only, no network, no inherited secrets.
type Sandbox struct {
	// Mode is "agent" (the agent's own operating-system sandbox; the default) or "none" (no such sandbox;
	// the agent still works in the worktree with a scrubbed environment, but can reach whatever the user's
	// account can). "none" is a decision for the person running the workflow, never a default.
	Mode    string   `yaml:"mode,omitempty"`
	Network bool     `yaml:"network,omitempty"`
	Env     []string `yaml:"env,omitempty"` // names of environment variables this step may see
}

// Step is one stage of a workflow.
type Step struct {
	ID          string   `yaml:"id"`
	Type        string   `yaml:"type"`
	Agent       string   `yaml:"agent,omitempty"`        // "any", a provider such as "codex", or "other-than:<step id>"
	Prompt      string   `yaml:"prompt,omitempty"`       // overrides the default prompt; may use {{task}} {{failure}} {{handoff}} {{diff}}
	Run         []string `yaml:"run,omitempty"`          // check commands, run in the worktree
	Timeout     Duration `yaml:"timeout,omitempty"`      // per attempt; 0 means the default
	MaxAttempts int      `yaml:"max_attempts,omitempty"` // for agent steps that fail for reasons other than a usage limit
	OnFail      *OnFail  `yaml:"on_fail,omitempty"`
	Sandbox     Sandbox  `yaml:"sandbox,omitempty"`
	AllowEmpty  bool     `yaml:"allow_empty,omitempty"` // an agent step may finish without changing any file (investigations)
}

// Unsandboxed reports whether the step asks to run without the agent's own sandbox.
func (s Step) Unsandboxed() bool { return s.Sandbox.Mode == "none" }

// Kind is the step's type as the shared model type.
func (s Step) Kind() model.StepKind { return model.StepKind(s.Type) }

// Workflow is a named list of steps.
type Workflow struct {
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// Parse reads a workflow and checks it. Every problem is reported at once.
func Parse(data []byte) (*Workflow, error) {
	var w Workflow
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("workflow: %w", err)
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	return &w, nil
}

// Load reads a workflow file.
func Load(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Validate reports every problem found, one per line.
func (w *Workflow) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if strings.TrimSpace(w.Name) == "" {
		add("name is required")
	}
	if len(w.Steps) == 0 {
		add("at least one step is required")
	}
	seen := map[string]int{}
	for i, s := range w.Steps {
		where := fmt.Sprintf("step %d", i+1)
		if s.ID != "" {
			where = fmt.Sprintf("step %q", s.ID)
		}
		if !idPattern.MatchString(s.ID) {
			add("%s: id must be lowercase letters, digits, - or _ and start with a letter", where)
		} else if _, dup := seen[s.ID]; dup {
			add("%s: duplicate id", where)
		}
		seen[s.ID] = i
		switch s.Kind() {
		case model.StepAgent:
			if s.Agent == "" {
				add("%s: agent steps need an agent (any, a provider name, or other-than:<step>)", where)
			}
		case model.StepCheck:
			if len(s.Run) == 0 {
				add("%s: check steps need at least one command in run", where)
			}
			for _, c := range s.Run {
				if strings.TrimSpace(c) == "" {
					add("%s: empty check command", where)
				}
			}
		case model.StepReview:
			if s.Agent == "" {
				add("%s: review steps need an agent (use other-than:<step> for an independent reviewer)", where)
			}
		default:
			add("%s: unknown type %q (agent, check or review)", where, s.Type)
		}
		if s.Agent != "" {
			if ref, ok := strings.CutPrefix(s.Agent, "other-than:"); ok {
				j, found := seen[ref]
				if !found || j >= i {
					add("%s: other-than:%s must name an earlier step", where, ref)
				} else if w.Steps[j].Kind() != model.StepAgent {
					add("%s: other-than:%s must name an agent step", where, ref)
				}
			}
		}
		if m := s.Sandbox.Mode; m != "" && m != "agent" && m != "none" {
			add("%s: sandbox mode must be agent or none, not %q", where, m)
		}
		if s.Timeout < 0 {
			add("%s: timeout must not be negative", where)
		}
		if s.MaxAttempts < 0 {
			add("%s: max_attempts must not be negative", where)
		}
		if s.OnFail != nil {
			if s.Kind() == model.StepAgent {
				add("%s: on_fail applies to check and review steps", where)
			}
			j, found := seen[s.OnFail.RetryWith]
			switch {
			case s.OnFail.RetryWith == "":
				add("%s: on_fail needs retry_with", where)
			case !found || j >= i:
				add("%s: retry_with %q must name an earlier step", where, s.OnFail.RetryWith)
			case w.Steps[j].Kind() != model.StepAgent:
				add("%s: retry_with %q must name an agent step", where, s.OnFail.RetryWith)
			}
			if s.OnFail.Max < 0 {
				add("%s: on_fail max must not be negative", where)
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("workflow %q is not valid:\n  - %s", w.Name, strings.Join(problems, "\n  - "))
	}
	return nil
}

// Default builds the workflow used when none is given: one agent step, then the checks the user passed,
// then an independent review when asked for.
func Default(checks []string, review bool) *Workflow {
	w := &Workflow{Name: "default", Steps: []Step{{ID: "implement", Type: "agent", Agent: "any"}}}
	if len(checks) > 0 {
		w.Steps = append(w.Steps, Step{
			ID: "checks", Type: "check", Run: checks,
			OnFail: &OnFail{RetryWith: "implement", Max: 2},
		})
	}
	if review {
		w.Steps = append(w.Steps, Step{
			ID: "review", Type: "review", Agent: "other-than:implement",
			OnFail: &OnFail{RetryWith: "implement", Max: 1},
		})
	}
	return w
}

// Marshal renders the workflow as YAML that Parse reads back.
func (w *Workflow) Marshal() ([]byte, error) { return yaml.Marshal(w) }

// StepIndex returns the position of the step with this id, or -1.
func (w *Workflow) StepIndex(id string) int {
	for i, s := range w.Steps {
		if s.ID == id {
			return i
		}
	}
	return -1
}
