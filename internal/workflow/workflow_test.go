package workflow

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

const good = `
name: ship
steps:
  - id: implement
    type: agent
    agent: any
    timeout: 20m
  - id: checks
    type: check
    run: ["go test ./...", "go vet ./..."]
    on_fail: {retry_with: implement, max: 2}
  - id: review
    type: review
    agent: other-than:implement
    on_fail: {retry_with: implement, max: 1}
`

func TestParseGoodWorkflow(t *testing.T) {
	w, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Steps) != 3 || w.Steps[0].Timeout != Duration(20*time.Minute) {
		t.Fatalf("parsed %+v", w)
	}
	if w.StepIndex("review") != 2 || w.StepIndex("nope") != -1 {
		t.Fatal("StepIndex")
	}
}

func TestValidationReportsEveryProblem(t *testing.T) {
	_, err := Parse([]byte(`
name: bad
steps:
  - id: Implement
    type: agent
  - id: checks
    type: check
  - id: checks
    type: nope
  - id: review
    type: review
    agent: other-than:later
    on_fail: {retry_with: review, max: 1}
  - id: later
    type: agent
    agent: any
`))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{
		"id must be lowercase",
		"agent steps need an agent",
		"check steps need at least one command",
		"duplicate id",
		"unknown type",
		"other-than:later must name an earlier step",
		`retry_with "review" must name an earlier step`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestUnknownFieldsAreRejected(t *testing.T) {
	_, err := Parse([]byte("name: x\nsteps:\n  - id: a\n    type: agent\n    agent: any\n    colour: red\n"))
	if err == nil || !strings.Contains(err.Error(), "colour") {
		t.Fatalf("err = %v", err)
	}
}

func TestOnFailBelongsToChecksAndReviews(t *testing.T) {
	_, err := Parse([]byte(`
name: x
steps:
  - id: a
    type: agent
    agent: any
  - id: b
    type: agent
    agent: any
    on_fail: {retry_with: a, max: 1}
`))
	if err == nil || !strings.Contains(err.Error(), "on_fail applies to check and review steps") {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultWorkflowIsValid(t *testing.T) {
	for _, tc := range []struct {
		checks []string
		review bool
		steps  int
	}{{nil, false, 1}, {[]string{"go test ./..."}, false, 2}, {[]string{"make"}, true, 3}, {nil, true, 2}} {
		w := Default(tc.checks, tc.review)
		if err := w.Validate(); err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if len(w.Steps) != tc.steps {
			t.Fatalf("%+v: %d steps", tc, len(w.Steps))
		}
	}
}

func TestAWorkflowSurvivesBeingSavedAndReadBack(t *testing.T) {
	w, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	w.Steps[0].AllowEmpty = true
	w.Steps[0].Sandbox = Sandbox{Network: true, Env: []string{"NPM_TOKEN"}}
	data, err := w.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(data)
	if err != nil {
		t.Fatalf("saved workflow does not parse: %v\n%s", err, data)
	}
	if len(back.Steps) != 3 || !back.Steps[0].AllowEmpty || back.Steps[0].Timeout != Duration(20*time.Minute) ||
		!back.Steps[0].Sandbox.Network || back.Steps[2].OnFail == nil || back.Steps[2].OnFail.RetryWith != "implement" {
		t.Fatalf("lost something on the round trip:\n%s", data)
	}
	if strings.Contains(string(data), "max_attempts") || strings.Contains(string(data), "prompt:") {
		t.Fatalf("empty fields should be left out:\n%s", data)
	}
}

func TestSandboxModeMustBeAgentOrNone(t *testing.T) {
	ok := "name: x\nsteps:\n  - id: a\n    type: agent\n    agent: any\n    sandbox: {mode: %s}\n"
	for _, mode := range []string{"agent", "none"} {
		if _, err := Parse([]byte(fmt.Sprintf(ok, mode))); err != nil {
			t.Errorf("mode %s rejected: %v", mode, err)
		}
	}
	_, err := Parse([]byte(fmt.Sprintf(ok, "off")))
	if err == nil || !strings.Contains(err.Error(), "sandbox mode must be agent or none") {
		t.Fatalf("err = %v", err)
	}
	w, _ := Parse([]byte(fmt.Sprintf(ok, "none")))
	if !w.Steps[0].Unsandboxed() {
		t.Fatal("Unsandboxed() is false for mode none")
	}
	w, _ = Parse([]byte(fmt.Sprintf(ok, "agent")))
	if w.Steps[0].Unsandboxed() {
		t.Fatal("Unsandboxed() is true for mode agent")
	}
}
