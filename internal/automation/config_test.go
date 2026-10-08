package automation

import (
	"runtime"
	"strings"
	"testing"
)

// absRoot is a full path on whichever system the tests run on (forward slashes work on Windows too).
func absRoot() string {
	if runtime.GOOS == "windows" {
		return "C:/work/app"
	}
	return "/work/app"
}

// cfg fills in the placeholder path.
func cfg(text string) []byte { return []byte(strings.ReplaceAll(text, "@ROOT@", absRoot())) }

const goodConfig = `
automations:
  - id: weekly-deps
    schedule: weekly mon 09:00
    repo: "@ROOT@"
    prompt: Update the dependencies and fix what breaks.
    checks: ["npm test"]
    review: true
    pr: draft
  - id: label-watch
    github:
      repo: Mvnshi/app
      label: legatus
      every: 15m
      authors: [Mvnshi]
    repo: "@ROOT@"
    checks: ["npm test"]
    pr: draft
    max_active: 1
`

func TestAGoodConfigParses(t *testing.T) {
	c, err := ParseConfig(cfg(goodConfig))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Automations) != 2 || c.Automations[1].GitHub.WatchEvery().Minutes() != 15 || c.Automations[1].maxActive() != 1 || c.Automations[0].maxActive() != 2 {
		t.Fatalf("config = %+v", c)
	}
}

func TestEveryProblemIsReportedAtOnce(t *testing.T) {
	_, err := ParseConfig(cfg(`
automations:
  - id: Bad Id
    schedule: sometimes
    repo: relative/path
  - id: both
    schedule: daily 09:00
    github: {repo: o/r, label: x, authors: [a]}
    repo: "@ROOT@"
    prompt: p
  - id: neither
    repo: "@ROOT@"
  - id: no-prompt
    schedule: daily 09:00
    repo: "@ROOT@"
  - id: open-watch
    github: {repo: not-a-slug, label: ""}
    repo: "@ROOT@"
  - id: odd-values
    schedule: daily 09:00
    prompt: p
    repo: "@ROOT@"
    pr: yes
    agent: gemini
    max_active: -1
  - id: both
    schedule: daily 09:00
    prompt: p
    repo: "@ROOT@"
`))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{
		"id must be 1-32 lowercase", `"sometimes" is not a schedule`, "repo must be a full path",
		"use either schedule or github, not both", "needs a schedule or a github watch", "needs a prompt",
		"github.repo must look like owner/name", "github.label is required", "github.authors is required",
		"pr must be draft or ready", "agent must be any, codex or claude", "max_active must not be negative", "duplicate id",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestAWatchNeedsAuthorsUnlessTheUserAcceptsTheRisk(t *testing.T) {
	base := "automations:\n  - id: w\n    repo: \"@ROOT@\"\n    github:\n      repo: o/r\n      label: legatus\n"
	if _, err := ParseConfig(cfg(base)); err == nil || !strings.Contains(err.Error(), "steer an agent on this machine") {
		t.Fatalf("a watch with no authors was accepted: %v", err)
	}
	if _, err := ParseConfig(cfg(base + "      anyone: true\n")); err != nil {
		t.Fatalf("anyone: true should be accepted: %v", err)
	}
	if _, err := ParseConfig(cfg(base + "      authors: [Mvnshi, app/github-actions, \"dependabot[bot]\"]\n")); err != nil {
		t.Fatalf("real login shapes rejected: %v", err)
	}
	if _, err := ParseConfig(cfg(base + "      authors: ['bad login!']\n")); err == nil {
		t.Fatal("a malformed login was accepted")
	}
}

func TestEmptyAndUnknownFields(t *testing.T) {
	if c, err := ParseConfig(nil); err != nil || len(c.Automations) != 0 {
		t.Fatalf("an empty file: %+v %v", c, err)
	}
	if _, err := ParseConfig([]byte("automations:\n  - id: a\n    colour: red\n")); err == nil || !strings.Contains(err.Error(), "colour") {
		t.Fatalf("an unknown field was accepted: %v", err)
	}
	if c, err := LoadConfig("/definitely/not/here.yaml"); err != nil || len(c.Automations) != 0 {
		t.Fatalf("a missing file should be an empty configuration: %v", err)
	}
}
