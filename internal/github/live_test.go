package github

import (
	"context"
	"os"
	"strings"
	"testing"
)

// A read-only check against real GitHub through the real gh. Skipped unless LEGATUS_LIVE_GH is set, because
// it needs network access and a signed-in gh. It reads one public issue and lists open ones; it changes
// nothing.
func TestLiveGHReadsARealIssue(t *testing.T) {
	if os.Getenv("LEGATUS_LIVE_GH") == "" {
		t.Skip("set LEGATUS_LIVE_GH=1 to run against real GitHub")
	}
	c := &Client{}
	issue, err := c.Issue(context.Background(), "Mvnshi/codex-subscription-router#14", "")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Ref.Number != 14 || issue.Title == "" || issue.Body == "" || len(issue.Labels) == 0 || issue.URL == "" {
		t.Fatalf("issue = %+v", issue)
	}
	prompt := TaskPrompt(issue)
	if !strings.Contains(prompt, "<issue>") || !strings.Contains(prompt, issue.Title) {
		t.Fatalf("prompt:\n%s", prompt)
	}
	list, err := c.ListIssues(context.Background(), "Mvnshi/codex-subscription-router", "upstream-build", 5)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read issue #%d %q (%d labels); %d open issue(s) carry the upstream-build label", issue.Ref.Number, issue.Title, len(issue.Labels), len(list))
}
