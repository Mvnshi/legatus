package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/fake"
	"github.com/Mvnshi/legatus/internal/github"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/pool"
	"github.com/Mvnshi/legatus/internal/store"
	"github.com/Mvnshi/legatus/internal/workflow"
	"github.com/Mvnshi/legatus/internal/worktree"
)

type tclock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *tclock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Sleep moves the clock forward instead of waiting, so a run that parks for hours finishes at once.
func (c *tclock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	if d > 0 {
		c.t = c.t.Add(d)
	}
	c.mu.Unlock()
	return ctx.Err()
}

type harness struct {
	t     *testing.T
	repo  string
	eng   *Engine
	clock *tclock
	pool  *pool.Pool
	store *store.Store
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func newHarness(t *testing.T, accounts []pool.Account, backends ...*fake.Backend) *harness {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q", "-b", "main")
	gitIn(t, repo, "config", "user.name", "Tester")
	gitIn(t, repo, "config", "user.email", "tester@example.com")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-q", "-m", "first")

	data := t.TempDir()
	st, err := store.Open(filepath.Join(data, "state"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &tclock{t: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	p := pool.New()
	p.Now = clock.Now
	for _, a := range accounts {
		if err := p.Add(a); err != nil {
			t.Fatal(err)
		}
	}
	bs := map[string]agent.Backend{}
	for _, b := range backends {
		bs[b.Provider()] = b
	}
	eng := &Engine{
		Store: st, Pool: p, Worktrees: worktree.New(), Backends: bs, DataDir: data,
		Now: clock.Now, Sleep: clock.Sleep,
		Environ: func() []string { return append(os.Environ(), "GITHUB_TOKEN=must-not-reach-the-agent") },
	}
	return &harness{t: t, repo: repo, eng: eng, clock: clock, pool: p, store: st}
}

func (h *harness) run(wf *workflow.Workflow, prompt string) *model.Run {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	run, err := h.eng.NewRun(ctx, model.Task{Prompt: prompt, Repo: h.repo, Base: "main"}, wf)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := h.eng.Execute(ctx, run.ID, wf); err != nil {
		h.t.Fatalf("Execute: %v", err)
	}
	got, err := h.store.Load(run.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return got
}

func (h *harness) events(run *model.Run) []model.Event {
	evs, _, err := h.store.Events(run.ID, 0)
	if err != nil {
		h.t.Fatal(err)
	}
	return evs
}

func hasEvent(evs []model.Event, typ string) bool {
	for _, e := range evs {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func committedFiles(t *testing.T, run *model.Run) string {
	t.Helper()
	return gitIn(t, run.Worktree, "log", "--name-only", "--format=%s", run.BaseCommit+"..HEAD")
}

func codexAccounts(ids ...string) []pool.Account {
	var out []pool.Account
	for _, id := range ids {
		out = append(out, pool.Account{ID: id, Provider: "codex", Home: "/pool/" + id})
	}
	return out
}

func TestASimpleRunSucceedsAndCommitsTheWork(t *testing.T) {
	be := fake.New("codex", nil)
	h := newHarness(t, codexAccounts("a"), be)
	run := h.run(workflow.Default([]string{"git --version"}, false), "add notes")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	if !strings.Contains(committedFiles(t, run), "notes-a-1.txt") {
		t.Fatalf("the agent's file is not committed on the branch:\n%s", committedFiles(t, run))
	}
	if run.Steps[0].Account != "a" || run.Steps[1].Summary == "" {
		t.Fatalf("steps = %+v", run.Steps)
	}
	if _, err := os.Stat(filepath.Join(h.repo, "notes-a-1.txt")); !os.IsNotExist(err) {
		t.Fatal("the work leaked into the original checkout")
	}
	data, err := os.ReadFile(filepath.Join(h.store.Dir(), "runs", run.ID, "evidence.md"))
	if err != nil || !strings.Contains(string(data), "**succeeded**") {
		t.Fatalf("evidence.md: %v\n%s", err, data)
	}
}

// A usage limit does not stop the workflow: the work moves to another login and every remaining step still runs.
func TestUsageLimitMidTaskContinuesOnAnotherLoginAndFinishesTheWorkflow(t *testing.T) {
	var h *harness
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if req.Account.ID == "a" {
			os.WriteFile(filepath.Join(req.Dir, "half-done.txt"), []byte("partial\n"), 0o600)
			return agent.Result{}, &agent.LimitError{ResetAt: h.clock.Now().Add(2 * time.Hour), Message: "You've hit your usage limit."}
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	h = newHarness(t, codexAccounts("a", "b"), be)
	// Make the first attempt go to "a" deterministically.
	h.pool.SetUsage("a", 100)
	h.pool.SetUsage("b", 10)

	run := h.run(workflow.Default([]string{"git --version"}, false), "build the thing")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	if run.Steps[0].Account != "b" || run.Steps[0].Attempts != 2 {
		t.Fatalf("implement step = %+v, want finished by b on its second attempt", run.Steps[0])
	}
	if run.Steps[1].Status != model.Succeeded {
		t.Fatalf("the check after the limit did not run: %+v", run.Steps[1])
	}
	calls := be.Calls()
	if len(calls) != 2 || calls[1].Account != "b" {
		t.Fatalf("calls = %+v", calls)
	}
	if !strings.Contains(calls[1].Prompt, "ran out of usage") || !strings.Contains(calls[1].Prompt, "half-done.txt") || !strings.Contains(calls[1].Prompt, "Do not start over") {
		t.Fatalf("the second login was not told what the first had done:\n%s", calls[1].Prompt)
	}
	if strings.Contains(calls[0].Prompt, "ran out of usage") {
		t.Fatal("the first attempt should not mention an interruption")
	}
	files := committedFiles(t, run)
	if !strings.Contains(files, "half-done.txt") || !strings.Contains(files, "notes-b-1.txt") {
		t.Fatalf("both the interrupted and the finished work belong on the branch:\n%s", files)
	}
	evs := h.events(run)
	if !hasEvent(evs, "account.limited") {
		t.Fatal("no account.limited event")
	}
	for _, s := range h.pool.Snapshots() {
		if s.ID == "a" && s.LimitedUntil.IsZero() {
			t.Fatal("account a was not set aside")
		}
	}
	ev, _ := os.ReadFile(filepath.Join(h.store.Dir(), "runs", run.ID, "evidence.md"))
	if !strings.Contains(string(ev), "`a` (codex) hit its usage limit") {
		t.Fatalf("evidence does not mention the limit:\n%s", ev)
	}
}

func TestWithEveryLoginOutTheRunWaitsForTheResetAndThenContinues(t *testing.T) {
	var h *harness
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if call == 1 {
			return agent.Result{}, &agent.LimitError{ResetAt: h.clock.Now().Add(3 * time.Hour), Message: "limit"}
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	h = newHarness(t, codexAccounts("only"), be)
	start := h.clock.Now()

	run := h.run(workflow.Default([]string{"git --version"}, false), "task")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	if waited := h.clock.Now().Sub(start); waited < 3*time.Hour {
		t.Fatalf("the run did not wait for the reset: only %s passed", waited)
	}
	evs := h.events(run)
	if !hasEvent(evs, "run.waiting_capacity") || !hasEvent(evs, "run.capacity_back") {
		t.Fatalf("expected waiting and back events, got %v", evs)
	}
	if run.Steps[1].Status != model.Succeeded {
		t.Fatalf("the rest of the workflow did not run after the wait: %+v", run.Steps)
	}
}

func TestAFailedCheckSendsTheAgentBackWithTheOutputAndThenPasses(t *testing.T) {
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if strings.Contains(req.Prompt, "Fix this") {
			os.WriteFile(filepath.Join(req.Dir, "fix.txt"), []byte("fixed\n"), 0o600)
			return agent.Result{Summary: "fixed it"}, nil
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	h := newHarness(t, codexAccounts("a"), be)
	wf := workflow.Default([]string{"git cat-file -e HEAD:fix.txt"}, false)
	run := h.run(wf, "make fix.txt exist")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	if run.Retries["checks"] != 1 {
		t.Fatalf("retries = %v", run.Retries)
	}
	calls := be.Calls()
	if len(calls) != 2 || !strings.Contains(calls[1].Prompt, "git cat-file -e HEAD:fix.txt") || !strings.Contains(calls[1].Prompt, "Fix this") {
		t.Fatalf("the retry did not carry the failure:\n%v", calls)
	}
	if !hasEvent(h.events(run), "step.sent_back") {
		t.Fatal("no step.sent_back event")
	}
}

func TestAFailedCheckWithNoRetriesLeftFailsTheRun(t *testing.T) {
	be := fake.New("codex", nil)
	h := newHarness(t, codexAccounts("a"), be)
	wf := workflow.Default([]string{"git cat-file -e HEAD:never.txt"}, false)
	wf.Steps[1].OnFail.Max = 1
	run := h.run(wf, "task")
	if run.Status != model.Failed || !strings.Contains(run.Error, "git cat-file -e HEAD:never.txt") || !strings.Contains(run.Error, "sent back 1 time") {
		t.Fatalf("status %s, error %q", run.Status, run.Error)
	}
	if len(be.Calls()) != 2 {
		t.Fatalf("agent ran %d times, want 2 (the original and one retry)", len(be.Calls()))
	}
}

func reviewer(verdicts ...string) fake.Handler {
	i := 0
	return func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		v := verdicts[min(i, len(verdicts)-1)]
		i++
		return agent.Result{Summary: "Looked at the diff.\n" + v}, nil
	}
}

func TestAnIndependentReviewerOnAnotherProviderApprovesReadOnly(t *testing.T) {
	author := fake.New("codex", nil)
	judge := fake.New("claude", reviewer(`{"verdict":"approve","summary":"matches the task","issues":[]}`))
	accounts := append(codexAccounts("a"), pool.Account{ID: "c", Provider: "claude"})
	h := newHarness(t, accounts, author, judge)
	run := h.run(workflow.Default(nil, true), "add notes")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	calls := judge.Calls()
	if len(calls) != 1 || !calls[0].Sandbox.ReadOnly || calls[0].Role != "review" {
		t.Fatalf("reviewer calls = %+v", calls)
	}
	if !strings.Contains(calls[0].Prompt, "notes-a-1.txt") || !strings.Contains(calls[0].Prompt, "must not modify") {
		t.Fatalf("the reviewer was not shown the change:\n%s", calls[0].Prompt)
	}
	if len(author.Calls()) != 1 {
		t.Fatal("the reviewer must not run on the author's provider when another exists")
	}
	if !strings.Contains(run.Steps[1].Summary, "other-provider") {
		t.Fatalf("review summary = %q", run.Steps[1].Summary)
	}
}

func TestReviewRequestingChangesSendsTheAgentBackOnce(t *testing.T) {
	author := fake.New("codex", nil)
	judge := fake.New("claude", reviewer(
		`{"verdict":"request_changes","summary":"missing a test","issues":["add a test for the empty case"]}`,
		`{"verdict":"approve","summary":"good now","issues":[]}`,
	))
	accounts := append(codexAccounts("a"), pool.Account{ID: "c", Provider: "claude"})
	h := newHarness(t, accounts, author, judge)
	run := h.run(workflow.Default(nil, true), "add notes")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	calls := author.Calls()
	if len(calls) != 2 || !strings.Contains(calls[1].Prompt, "add a test for the empty case") {
		t.Fatalf("the author did not receive the reviewer's issues: %+v", calls)
	}
	if len(judge.Calls()) != 2 {
		t.Fatalf("reviewer ran %d times, want 2", len(judge.Calls()))
	}
}

func TestReviewThatKeepsRequestingChangesHandsTheRunToAPerson(t *testing.T) {
	author := fake.New("codex", nil)
	judge := fake.New("claude", reviewer(`{"verdict":"request_changes","summary":"still wrong","issues":["x"]}`))
	accounts := append(codexAccounts("a"), pool.Account{ID: "c", Provider: "claude"})
	h := newHarness(t, accounts, author, judge)
	run := h.run(workflow.Default(nil, true), "add notes")
	if run.Status != model.NeedsHuman || !strings.Contains(run.Error, "still wrong") {
		t.Fatalf("status %s, error %q", run.Status, run.Error)
	}
}

func TestAReviewWithoutAVerdictIsNeverTreatedAsApproval(t *testing.T) {
	author := fake.New("codex", nil)
	judge := fake.New("claude", reviewer("Looks fine to me!"))
	accounts := append(codexAccounts("a"), pool.Account{ID: "c", Provider: "claude"})
	h := newHarness(t, accounts, author, judge)
	run := h.run(workflow.Default(nil, true), "add notes")
	if run.Status != model.NeedsHuman || !strings.Contains(run.Error, "did not return a verdict") {
		t.Fatalf("status %s, error %q", run.Status, run.Error)
	}
}

func TestReviewUsesAnotherLoginOfTheSameProviderWhenThatIsAllThereIs(t *testing.T) {
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if req.Role == "review" {
			return agent.Result{Summary: `{"verdict":"approve","summary":"ok"}`}, nil
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	h := newHarness(t, codexAccounts("a", "b"), be)
	h.pool.SetUsage("a", 100)
	h.pool.SetUsage("b", 1)
	run := h.run(workflow.Default(nil, true), "add notes")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	if run.Steps[0].Account != "a" || run.Steps[1].Account != "b" || !strings.Contains(run.Steps[1].Summary, "other-account") {
		t.Fatalf("steps = %+v", run.Steps)
	}
}

func TestReviewWithOnlyOneLoginSaysSoInTheEvidence(t *testing.T) {
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if req.Role == "review" {
			return agent.Result{Summary: `{"verdict":"approve","summary":"ok"}`}, nil
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	h := newHarness(t, codexAccounts("only"), be)
	run := h.run(workflow.Default(nil, true), "add notes")
	if run.Status != model.Succeeded || !strings.Contains(run.Steps[1].Summary, "same-account") {
		t.Fatalf("status %s, review = %q", run.Status, run.Steps[1].Summary)
	}
}

// A review step that does not say "other-than:implement" (agent: any, or a named provider) used to record
// an empty independence, which showed up as "approve ()" and "(): ..." in the summary and evidence.md.
func TestAReviewThatDidNotAskForIndependenceStillSaysHowIndependentItWas(t *testing.T) {
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if req.Role == "review" {
			return agent.Result{Summary: `{"verdict":"approve","summary":"ok"}`}, nil
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	h := newHarness(t, codexAccounts("only"), be)
	wf := workflow.Default(nil, true)
	wf.Steps[len(wf.Steps)-1].Agent = "any"
	run := h.run(wf, "add notes")
	review := run.Steps[len(run.Steps)-1]
	if run.Status != model.Succeeded || strings.Contains(review.Summary, "()") || !strings.Contains(review.Summary, "same-account") {
		t.Fatalf("status %s, review = %q", run.Status, review.Summary)
	}
	evidence, err := os.ReadFile(filepath.Join(h.store.Dir(), "runs", run.ID, "evidence.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(evidence), "()") || !strings.Contains(string(evidence), "(same-account)") {
		t.Fatalf("evidence.md names no independence:\n%s", evidence)
	}
}

func TestSecretsInTheTaskNeverReachTheAgentAndTheEnvironmentIsScrubbed(t *testing.T) {
	be := fake.New("codex", nil)
	h := newHarness(t, codexAccounts("a"), be)
	secret := "gh" + "p_" + strings.Repeat("Q9z", 12)
	run := h.run(workflow.Default(nil, false), "Deploy using token "+secret+" please")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	call := be.Calls()[0]
	if strings.Contains(call.Prompt, secret) || !strings.Contains(call.Prompt, "[REDACTED:github-token]") {
		t.Fatalf("prompt was not redacted:\n%s", call.Prompt)
	}
	for _, kv := range call.Env {
		if strings.HasPrefix(strings.ToUpper(kv), "GITHUB_TOKEN=") {
			t.Fatal("GITHUB_TOKEN reached the agent")
		}
	}
	found := false
	for _, kv := range call.Env {
		if kv == "FAKE_AGENT_HOME=/pool/a" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the account's own settings were not passed: %v", call.Env)
	}
	for _, ev := range h.events(run) {
		if b, _ := os.ReadFile(filepath.Join(h.store.Dir(), "runs", run.ID, "events.jsonl")); strings.Contains(string(b), secret) {
			t.Fatalf("the secret reached the journal (event %s)", ev.Type)
		}
	}
}

func TestCancellingLeavesTheRunResumable(t *testing.T) {
	block := make(chan struct{})
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if call == 1 {
			close(block)
			<-ctx.Done()
			return agent.Result{}, ctx.Err()
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	h := newHarness(t, codexAccounts("a"), be)
	wf := workflow.Default([]string{"git --version"}, false)
	run, err := h.eng.NewRun(context.Background(), model.Task{Prompt: "task", Repo: h.repo, Base: "main"}, wf)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.eng.Execute(ctx, run.ID, wf) }()
	<-block
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute = %v", err)
	}
	mid, _ := h.store.Load(run.ID)
	if mid.Status != model.Running || mid.Current != 0 {
		t.Fatalf("after cancel: %s at step %d", mid.Status, mid.Current)
	}
	if err := h.eng.Execute(context.Background(), run.ID, wf); err != nil {
		t.Fatal(err)
	}
	end, _ := h.store.Load(run.ID)
	if end.Status != model.Succeeded {
		t.Fatalf("after resume: %s: %s", end.Status, end.Error)
	}
	// The account was handed back, so the resumed run could use it.
	for _, s := range h.pool.Snapshots() {
		if s.Active != 0 {
			t.Fatalf("account %s still leased", s.ID)
		}
	}
}

func TestAgentErrorsRetryOnceThenFail(t *testing.T) {
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		return agent.Result{}, errors.New("segfault in the agent")
	})
	h := newHarness(t, codexAccounts("a"), be)
	run := h.run(workflow.Default(nil, false), "task")
	if run.Status != model.Failed || !strings.Contains(run.Error, "failed 2 times") || !strings.Contains(run.Error, "segfault") {
		t.Fatalf("status %s, error %q", run.Status, run.Error)
	}
	if got := be.Calls(); len(got) != 2 || !strings.Contains(got[1].Prompt, "segfault") {
		t.Fatalf("the retry did not say what went wrong: %+v", got)
	}
	for _, s := range h.pool.Snapshots() {
		if s.Active != 0 || !s.LimitedUntil.IsZero() {
			t.Fatalf("an agent error must not bench the account: %+v", s)
		}
	}
}

func TestNoAccountsGivesAClearFailure(t *testing.T) {
	h := newHarness(t, nil, fake.New("codex", nil))
	run := h.run(workflow.Default(nil, false), "task")
	if run.Status != model.Failed || !strings.Contains(run.Error, "legatus accounts add") {
		t.Fatalf("status %s, error %q", run.Status, run.Error)
	}
}

func TestManyRunsAtOnceShareTheLoginsAndTheRepository(t *testing.T) {
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		time.Sleep(5 * time.Millisecond)
		return fake.WriteFile(ctx, req, call, emit)
	})
	h := newHarness(t, codexAccounts("a", "b", "c"), be)
	wf := workflow.Default([]string{"git --version"}, false)
	const n = 9
	var wg sync.WaitGroup
	runs := make([]*model.Run, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			r, err := h.eng.NewRun(ctx, model.Task{Prompt: fmt.Sprintf("task %d", i), Repo: h.repo, Base: "main"}, wf)
			if err != nil {
				t.Errorf("NewRun %d: %v", i, err)
				return
			}
			if err := h.eng.Execute(ctx, r.ID, wf); err != nil {
				t.Errorf("Execute %d: %v", i, err)
			}
			runs[i], _ = h.store.Load(r.ID)
		}(i)
	}
	wg.Wait()
	used := map[string]bool{}
	for i, r := range runs {
		if r == nil || r.Status != model.Succeeded {
			t.Fatalf("run %d: %+v", i, r)
		}
		used[r.Steps[0].Account] = true
	}
	if len(used) != 3 {
		t.Fatalf("work did not spread over all three logins: %v", used)
	}
}

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{`ok {"verdict":"approve","summary":"s"}`, "approve", true},
		{"```json\n{\"verdict\":\"Request_Changes\",\"issues\":[\"a\"]}\n```", "request_changes", true},
		{`first {"verdict":"approve"} then later {"verdict":"request_changes","summary":"no"}`, "request_changes", true},
		{`{"verdict":"maybe"}`, "", false},
		{`no json at all`, "", false},
		{`{"verdict":`, "", false},
		{``, "", false},
	}
	for _, c := range cases {
		v, ok := parseVerdict(c.in)
		if ok != c.ok || v.Verdict != c.want {
			t.Errorf("parseVerdict(%q) = %q, %v; want %q, %v", c.in, v.Verdict, ok, c.want, c.ok)
		}
	}
}

// Found by running the real Codex: an agent that cannot do the work often exits cleanly and says so in words.
func TestAnAgentThatChangedNothingIsNotASuccess(t *testing.T) {
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		return agent.Result{Summary: "I could not write the file: the workspace is read-only."}, nil
	})
	h := newHarness(t, codexAccounts("a"), be)
	run := h.run(workflow.Default([]string{"git --version"}, false), "create a file")
	if run.Status != model.Failed || !strings.Contains(run.Error, "without changing any file") || !strings.Contains(run.Error, "workspace is read-only") {
		t.Fatalf("status %s, error %q", run.Status, run.Error)
	}
	if run.Steps[1].Status == model.Succeeded {
		t.Fatal("the checks must not run after a step that did nothing")
	}
	if !hasEvent(h.events(run), "agent.no_changes") {
		t.Fatal("no agent.no_changes event")
	}
}

func TestAStepThatMayBeEmptyCanFinishWithoutChanges(t *testing.T) {
	be := fake.New("codex", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		return agent.Result{Summary: "There is no bug; nothing to change."}, nil
	})
	h := newHarness(t, codexAccounts("a"), be)
	wf := workflow.Default(nil, false)
	wf.Steps[0].AllowEmpty = true
	run := h.run(wf, "investigate whether there is a bug")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
}

func TestABrokenAgentSandboxStopsTheRunBeforeAnyWorkAndSaysHowToProceed(t *testing.T) {
	be := fake.New("codex", nil)
	be.PreflightErr = errors.New("Access is denied")
	h := newHarness(t, codexAccounts("a"), be)
	run := h.run(workflow.Default(nil, false), "task")
	if run.Status != model.Failed {
		t.Fatalf("status %s", run.Status)
	}
	for _, want := range []string{`sandbox for account "a" does not work`, "Access is denied", "--no-sandbox"} {
		if !strings.Contains(run.Error, want) {
			t.Errorf("error lacks %q: %s", want, run.Error)
		}
	}
	if len(be.Calls()) != 0 {
		t.Fatal("the agent ran even though its sandbox does not work")
	}
	// The answer is remembered: a second run does not pay for the check again.
	h.run(workflow.Default(nil, false), "another task")
	if be.Preflights != 1 {
		t.Fatalf("the sandbox was checked %d times, want once", be.Preflights)
	}
	var checked int
	for _, ev := range h.events(run) {
		if ev.Type == "sandbox.checked" && ev.Data["ok"] == false {
			checked++
		}
	}
	if checked != 1 {
		t.Fatalf("expected one failed sandbox.checked event, got %d", checked)
	}
}

func TestRunningWithoutTheAgentSandboxIsAnExplicitChoiceThatSkipsTheCheck(t *testing.T) {
	be := fake.New("codex", nil)
	be.PreflightErr = errors.New("Access is denied")
	h := newHarness(t, codexAccounts("a"), be)
	wf := workflow.Default([]string{"git --version"}, false)
	wf.Steps[0].Sandbox.Mode = "none"
	run := h.run(wf, "task")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	if be.Preflights != 0 {
		t.Fatal("the sandbox check ran for a step that opted out")
	}
	if calls := be.Calls(); len(calls) != 1 || !calls[0].Sandbox.Unrestricted {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestAReviewerThatTouchesFilesHasThemRevertedAndSaysSo(t *testing.T) {
	author := fake.New("codex", nil)
	judge := fake.New("claude", func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		os.WriteFile(filepath.Join(req.Dir, "reviewer-was-here.txt"), []byte("oops\n"), 0o600)
		os.WriteFile(filepath.Join(req.Dir, "README.md"), []byte("changed by the reviewer\n"), 0o600)
		return agent.Result{Summary: `{"verdict":"approve","summary":"fine"}`}, nil
	})
	accounts := append(codexAccounts("a"), pool.Account{ID: "c", Provider: "claude"})
	h := newHarness(t, accounts, author, judge)
	run := h.run(workflow.Default(nil, true), "add notes")
	if run.Status != model.Succeeded {
		t.Fatalf("status %s: %s", run.Status, run.Error)
	}
	if !hasEvent(h.events(run), "review.modified_files") || !strings.Contains(run.Steps[1].Summary, "they were reverted") {
		t.Fatalf("the reviewer's changes were not reported: %q", run.Steps[1].Summary)
	}
	if _, err := os.Stat(filepath.Join(run.Worktree, "reviewer-was-here.txt")); !os.IsNotExist(err) {
		t.Fatal("the reviewer's new file is still there")
	}
	// (git may check files out with Windows line endings, so compare the words, not the bytes)
	if data, _ := os.ReadFile(filepath.Join(run.Worktree, "README.md")); strings.TrimSpace(string(data)) != "hello" {
		t.Fatalf("the reviewer's edit to README.md survived: %q", data)
	}
	if strings.Contains(committedFiles(t, run), "reviewer-was-here") {
		t.Fatal("the reviewer's file was committed")
	}
}

type stubPR struct {
	calls []github.PROptions
	err   error
}

func (s *stubPR) OpenPR(ctx context.Context, o github.PROptions) (string, error) {
	s.calls = append(s.calls, o)
	if s.err != nil {
		return "", s.err
	}
	return "https://github.com/o/r/pull/9", nil
}

func TestAFinishedRunBecomesAPullRequestWithItsReportAsTheDescription(t *testing.T) {
	h := newHarness(t, codexAccounts("a"), fake.New("codex", nil))
	stub := &stubPR{}
	h.eng.GitHub = stub
	ctx := context.Background()
	run, err := h.eng.NewRun(ctx, model.Task{Prompt: "fix it", Repo: h.repo, Base: "main", Source: "github:o/r#33"}, workflow.Default([]string{"git --version"}, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.eng.OpenPullRequest(ctx, run.ID, false); err == nil || !strings.Contains(err.Error(), "once it has succeeded") {
		t.Fatalf("a run that has not run was given a pull request: %v", err)
	}
	if err := h.eng.Resume(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	url, err := h.eng.OpenPullRequest(ctx, run.ID, true)
	if err != nil || url != "https://github.com/o/r/pull/9" {
		t.Fatalf("url %q, err %v", url, err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("%d calls", len(stub.calls))
	}
	c := stub.calls[0]
	if c.Branch != "legatus/"+run.ID || c.Base != "main" || !c.Draft || c.Title != "fix it" || c.Dir == "" {
		t.Fatalf("options = %+v", c)
	}
	if !strings.HasPrefix(c.Body, "Closes o/r#33\n\n# fix it") || !strings.Contains(c.Body, "**succeeded**") {
		t.Fatalf("body:\n%s", c.Body)
	}
	got, _ := h.store.Load(run.ID)
	if got.PRURL != url || !hasEvent(h.events(got), "pr.opened") {
		t.Fatalf("the pull request was not recorded: %+v", got.PRURL)
	}
	// Asking again returns the same one and does not push or create anything.
	if again, err := h.eng.OpenPullRequest(ctx, run.ID, false); err != nil || again != url || len(stub.calls) != 1 {
		t.Fatalf("second call: %q, %v, %d calls", again, err, len(stub.calls))
	}
}

func TestPullRequestBasesAndNeedsHumanRunsAreDraft(t *testing.T) {
	for in, want := range map[string]string{"": "", "HEAD": "", "main": "main", "release/1.2": "release/1.2", "a1b2c3d": "", "0123456789abcdef0123456789abcdef01234567": "", "bad branch": "", "--force": "--force"} {
		if got := baseBranch(in); got != want {
			t.Errorf("baseBranch(%q) = %q, want %q", in, got, want)
		}
	}
	judge := fake.New("claude", reviewer(`{"verdict":"request_changes","summary":"still wrong","issues":["x"]}`))
	accounts := append(codexAccounts("a"), pool.Account{ID: "c", Provider: "claude"})
	h := newHarness(t, accounts, fake.New("codex", nil), judge)
	stub := &stubPR{}
	h.eng.GitHub = stub
	run := h.run(workflow.Default(nil, true), "add notes")
	if run.Status != model.NeedsHuman {
		t.Fatalf("status %s", run.Status)
	}
	if _, err := h.eng.OpenPullRequest(context.Background(), run.ID, false); err != nil {
		t.Fatal(err)
	}
	if !stub.calls[0].Draft {
		t.Fatal("a run a person has not agreed with must open as a draft")
	}
	if strings.Contains(stub.calls[0].Body, "Closes") {
		t.Fatal("an unreviewed run must not claim to close an issue")
	}
}

func TestAFailedPullRequestIsRecordedAndDoesNotChangeTheRun(t *testing.T) {
	h := newHarness(t, codexAccounts("a"), fake.New("codex", nil))
	h.eng.GitHub = &stubPR{err: errors.New("could not push legatus/x to origin: permission denied")}
	run := h.run(workflow.Default(nil, false), "task")
	if _, err := h.eng.OpenPullRequest(context.Background(), run.ID, false); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v", err)
	}
	got, _ := h.store.Load(run.ID)
	if got.Status != model.Succeeded || got.PRURL != "" || !hasEvent(h.events(got), "pr.failed") {
		t.Fatalf("status %s, pr %q", got.Status, got.PRURL)
	}
	failedRun := h.run(workflow.Default([]string{"git cat-file -e HEAD:never"}, false), "task two")
	if failedRun.Status != model.Failed {
		t.Skip("setup did not produce a failed run")
	}
	if _, err := h.eng.OpenPullRequest(context.Background(), failedRun.ID, false); err == nil {
		t.Fatal("a failed run was given a pull request")
	}
}

// Found by using Legatus on itself: several runs each building and testing at once used up a machine's memory.
func TestHeavyChecksRunOneAtATimeAcrossRunsAndTheLimitCanBeRaised(t *testing.T) {
	measure := func(slots int) (peak int32, runs []*model.Run, h *harness) {
		h = newHarness(t, codexAccounts("a", "b", "c", "d"), fake.New("codex", nil))
		h.eng.CheckSlots = slots
		var now int32
		h.eng.shell = func(ctx context.Context, dir, command string, env []string, timeout time.Duration) (string, int, error) {
			n := atomic.AddInt32(&now, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
					break
				}
			}
			time.Sleep(1200 * time.Millisecond)
			atomic.AddInt32(&now, -1)
			return "", 0, nil
		}
		wf := workflow.Default([]string{"a heavy command"}, false)
		runs = make([]*model.Run, 4)
		var wg sync.WaitGroup
		for i := range runs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				r, err := h.eng.NewRun(ctx, model.Task{Prompt: fmt.Sprintf("task %d", i), Repo: h.repo, Base: "main"}, wf)
				if err != nil {
					t.Errorf("NewRun: %v", err)
					return
				}
				if err := h.eng.Execute(ctx, r.ID, wf); err != nil {
					t.Errorf("Execute: %v", err)
				}
				runs[i], _ = h.store.Load(r.ID)
			}(i)
		}
		wg.Wait()
		return peak, runs, h
	}

	peak, runs, h := measure(0) // the default is one at a time
	if peak != 1 {
		t.Fatalf("%d check commands ran at once with the default limit, want 1", peak)
	}
	waited := false
	for _, r := range runs {
		if r == nil || r.Status != model.Succeeded {
			t.Fatalf("run = %+v", r)
		}
		waited = waited || hasEvent(h.events(r), "check.waiting")
	}
	if !waited {
		t.Fatal("no run said it was waiting for the check slot")
	}
	if peak, _, _ := measure(2); peak != 2 {
		t.Fatalf("%d check commands ran at once with two slots, want 2", peak)
	}
}
