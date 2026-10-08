package automation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Watch is a trigger that looks for GitHub issues.
type Watch struct {
	Repo    string   `yaml:"repo"`              // owner/name
	Label   string   `yaml:"label"`             // only issues carrying this label
	Every   string   `yaml:"every,omitempty"`   // how often to look; 10m when empty
	Authors []string `yaml:"authors,omitempty"` // only issues written by these logins
	Anyone  bool     `yaml:"anyone,omitempty"`  // accept any author (see the warning in the docs)
	MaxNew  int      `yaml:"max_new,omitempty"` // at most this many new runs per look; 2 when zero
}

// Automation is one standing instruction.
type Automation struct {
	ID        string   `yaml:"id"`
	Disabled  bool     `yaml:"disabled,omitempty"`
	Schedule  string   `yaml:"schedule,omitempty"`
	GitHub    *Watch   `yaml:"github,omitempty"`
	Repo      string   `yaml:"repo"` // the local checkout the work happens in
	Base      string   `yaml:"base,omitempty"`
	Prompt    string   `yaml:"prompt,omitempty"` // required for a schedule; a watch uses the issue
	Checks    []string `yaml:"checks,omitempty"`
	Review    bool     `yaml:"review,omitempty"`
	Agent     string   `yaml:"agent,omitempty"`
	PR        string   `yaml:"pr,omitempty"` // draft or ready: open a pull request when a run succeeds
	NoSandbox bool     `yaml:"no_sandbox,omitempty"`
	MaxActive int      `yaml:"max_active,omitempty"` // runs of this automation at once; 2 when zero
}

// Config is the whole file.
type Config struct {
	Automations []Automation `yaml:"automations"`
}

var (
	idPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	slugPat      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
	loginPat     = regexp.MustCompile(`^(app/)?[A-Za-z0-9][A-Za-z0-9-]*(\[bot\])?$`)
	defaultEvery = 10 * time.Minute
)

// ParseConfig reads and checks the file, reporting every problem at once.
func ParseConfig(data []byte) (*Config, error) {
	var c Config
	if len(strings.TrimSpace(string(data))) > 0 {
		dec := yaml.NewDecoder(strings.NewReader(string(data)))
		dec.KnownFields(true)
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("automations: %w", err)
		}
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// LoadConfig reads the file; a missing file is an empty configuration.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseConfig(data)
}

// WatchEvery is how often a watch looks.
func (w *Watch) WatchEvery() time.Duration {
	if w.Every == "" {
		return defaultEvery
	}
	d, err := time.ParseDuration(w.Every)
	if err != nil {
		return defaultEvery
	}
	return d
}

// Validate reports every problem, one per line.
func (c *Config) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	seen := map[string]bool{}
	for i, a := range c.Automations {
		where := fmt.Sprintf("automation %d", i+1)
		if a.ID != "" {
			where = fmt.Sprintf("automation %q", a.ID)
		}
		switch {
		case !idPattern.MatchString(a.ID):
			add("%s: id must be 1-32 lowercase letters, digits, - or _", where)
		case seen[a.ID]:
			add("%s: duplicate id", where)
		}
		seen[a.ID] = true
		if strings.TrimSpace(a.Repo) == "" {
			add("%s: repo (the local checkout) is required", where)
		} else if !filepath.IsAbs(a.Repo) {
			add("%s: repo must be a full path", where)
		}
		switch {
		case a.Schedule != "" && a.GitHub != nil:
			add("%s: use either schedule or github, not both", where)
		case a.Schedule == "" && a.GitHub == nil:
			add("%s: needs a schedule or a github watch", where)
		}
		if a.Schedule != "" {
			if _, err := ParseSchedule(a.Schedule); err != nil {
				add("%s: %v", where, err)
			}
			if strings.TrimSpace(a.Prompt) == "" {
				add("%s: a scheduled automation needs a prompt", where)
			}
		}
		if w := a.GitHub; w != nil {
			if !slugPat.MatchString(w.Repo) {
				add("%s: github.repo must look like owner/name", where)
			}
			if strings.TrimSpace(w.Label) == "" {
				add("%s: github.label is required (the label that marks an issue for Legatus)", where)
			}
			if w.Every != "" {
				if d, err := time.ParseDuration(w.Every); err != nil || d < time.Minute {
					add("%s: github.every must be a duration of at least 1m, like 10m", where)
				}
			}
			if len(w.Authors) == 0 && !w.Anyone {
				add("%s: github.authors is required. Anyone who can label or write an issue could otherwise steer an agent on this machine. List the logins you trust, or set anyone: true to accept the risk", where)
			}
			for _, au := range w.Authors {
				if !loginPat.MatchString(au) {
					add("%s: %q is not a GitHub login", where, au)
				}
			}
			if w.MaxNew < 0 {
				add("%s: github.max_new must not be negative", where)
			}
		}
		if a.PR != "" && a.PR != "draft" && a.PR != "ready" {
			add("%s: pr must be draft or ready", where)
		}
		if a.Agent != "" && a.Agent != "any" && a.Agent != "codex" && a.Agent != "claude" {
			add("%s: agent must be any, codex or claude", where)
		}
		if a.MaxActive < 0 {
			add("%s: max_active must not be negative", where)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("the automations file is not valid:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

func (a Automation) maxActive() int {
	if a.MaxActive > 0 {
		return a.MaxActive
	}
	return 2
}
