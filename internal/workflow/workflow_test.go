package workflow

import (
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
