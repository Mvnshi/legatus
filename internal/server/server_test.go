package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/fake"
	"github.com/Mvnshi/legatus/internal/app"
	"github.com/Mvnshi/legatus/internal/github"
	"github.com/Mvnshi/legatus/internal/hub"
	"github.com/Mvnshi/legatus/internal/pool"
	"github.com/Mvnshi/legatus/internal/queue"
)

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type env struct {
	t    *testing.T
	srv  *Server
	ts   *httptest.Server
	repo string
	app  *app.App
	be   *fake.Backend
}

func newEnv(t *testing.T, handler fake.Handler) *env {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "T"}, {"config", "user.email", "t@example.com"}} {
		run(t, repo, args...)
	}
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o600)
	run(t, repo, "add", "-A")
	run(t, repo, "commit", "-q", "-m", "first")

	be := fake.New("codex", handler)
	a, err := app.Open(t.TempDir(), app.Options{Backends: map[string]agent.Backend{"codex": be}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddAccount("main", "codex", "default", "", 4); err != nil {
		t.Fatal(err)
	}
	h := hub.New()
	sched := &queue.Scheduler{Engine: a.Engine, Hub: h, Concurrency: 2}
	if _, err := sched.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sched.Stop)
	s := &Server{App: a, Sched: sched, Hub: h, Token: testToken, Version: "test"}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &env{t: t, srv: s, ts: ts, repo: repo, app: a, be: be}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (e *env) do(method, path string, body any, mutate ...func(*http.Request)) (int, []byte) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		rd = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, rd)
	req.Header.Set("X-Legatus-Token", testToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, m := range mutate {
		m(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func (e *env) waitStatus(id string, want ...string) map[string]any {
	e.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		code, data := e.do("GET", "/api/runs/"+id, nil)
		if code == 200 {
			var r struct {
				Summary map[string]any `json:"summary"`
			}
			json.Unmarshal(data, &r)
			for _, w := range want {
				if r.Summary["status"] == w {
					return r.Summary
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	e.t.Fatalf("run %s never reached %v", id, want)
	return nil
}

func TestTheGuardRejectsStrangers(t *testing.T) {
	e := newEnv(t, nil)
	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		{"no token", func(r *http.Request) { r.Header.Del("X-Legatus-Token") }, 401},
		{"wrong token", func(r *http.Request) { r.Header.Set("X-Legatus-Token", strings.Repeat("f", 64)) }, 401},
		{"another host name (DNS rebinding)", func(r *http.Request) { r.Host = "evil.example:7420" }, 403},
		{"another website's page", func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }, 403},
		{"an unrelated address", func(r *http.Request) { r.Host = "192.168.1.20:7420" }, 403},
		{"the right host and token", func(r *http.Request) {}, 200},
		{"localhost works too", func(r *http.Request) { r.Host = "localhost:7420" }, 200},
	}
	for _, c := range cases {
		if code, _ := e.do("GET", "/api/health", nil, c.mutate); code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, code, c.want)
		}
	}
	// A page from another site cannot even read the cockpit's files under a foreign host name.
	req, _ := http.NewRequest("GET", e.ts.URL+"/", nil)
	req.Host = "evil.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("the cockpit answered a foreign host with %d", resp.StatusCode)
	}
}

func TestTheCockpitIsServedWithAStrictPolicy(t *testing.T) {
	e := newEnv(t, nil)
	for path, want := range map[string]string{"/": "text/html", "/app.js": "javascript", "/style.css": "text/css"} {
		resp, err := http.Get(e.ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), want) || len(body) == 0 {
			t.Errorf("%s: %d %q", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		csp := resp.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
			t.Errorf("%s: policy %q", path, csp)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", path)
		}
	}
	// The page and script must not rely on inline code, or the policy would block them.
	resp, _ := http.Get(e.ts.URL + "/")
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(page), "<script>") || strings.Contains(string(page), "onclick=") {
		t.Error("the page has inline script")
	}
	js, _ := http.Get(e.ts.URL + "/app.js")
	src, _ := io.ReadAll(js.Body)
	js.Body.Close()
	if strings.Contains(string(src), "innerHTML") {
		t.Error("app.js uses innerHTML; untrusted run text could inject markup")
	}
	// The policy forbids inline style attributes, so these would silently do nothing in a browser.
	if strings.Contains(string(page), " style=") || strings.Contains(string(src), "setAttribute('style'") || strings.Contains(string(src), "cssText") {
		t.Error("the page uses inline styles, which the content policy blocks")
	}
}

func TestSubmitWatchAndReadARun(t *testing.T) {
	e := newEnv(t, nil)
	code, data := e.do("POST", "/api/runs", map[string]any{
		"prompt": "add notes", "repo": e.repo, "base": "main", "checks": []string{"git --version"}, "agent": "codex",
	})
	if code != 201 {
		t.Fatalf("submit: %d %s", code, data)
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(data, &created)
	sum := e.waitStatus(created.ID, "succeeded", "failed")
	if sum["status"] != "succeeded" {
		t.Fatalf("run ended %v (%v)", sum["status"], sum["error"])
	}

	code, data = e.do("GET", "/api/runs/"+created.ID, nil)
	var full struct {
		Events   []map[string]any `json:"events"`
		Evidence string           `json:"evidence"`
	}
	json.Unmarshal(data, &full)
	if code != 200 || len(full.Events) < 5 || !strings.Contains(full.Evidence, "**succeeded**") {
		t.Fatalf("detail: %d, %d events, evidence %q", code, len(full.Events), full.Evidence)
	}

	code, data = e.do("GET", "/api/runs", nil)
	var list []map[string]any
	json.Unmarshal(data, &list)
	if code != 200 || len(list) != 1 || list[0]["id"] != created.ID {
		t.Fatalf("list: %d %s", code, data)
	}
	if code, _ := e.do("GET", "/api/runs?status=failed", nil); code != 200 {
		t.Fatalf("filter: %d", code)
	}

	code, data = e.do("GET", "/api/runs/"+created.ID+"/diff", nil)
	if code != 200 || !strings.Contains(string(data), "notes-main-1.txt") {
		t.Fatalf("diff: %d %s", code, data)
	}

	code, data = e.do("GET", "/api/state", nil)
	var st struct {
		Counts      map[string]int `json:"counts"`
		Accounts    []pool.Snapshot
		RecentRepos []string `json:"recent_repos"`
	}
	json.Unmarshal(data, &st)
	if code != 200 || st.Counts["succeeded"] != 1 || len(st.Accounts) != 1 || len(st.RecentRepos) != 1 {
		t.Fatalf("state: %d %s", code, data)
	}
}

func TestSubmitValidatesTheRequest(t *testing.T) {
	e := newEnv(t, nil)
	for name, body := range map[string]any{
		"no prompt":      map[string]any{"repo": e.repo},
		"no repo":        map[string]any{"prompt": "x"},
		"not a repo":     map[string]any{"prompt": "x", "repo": t.TempDir()},
		"unknown field":  map[string]any{"prompt": "x", "repo": e.repo, "rm_rf": true},
		"bad workflow":   map[string]any{"prompt": "x", "repo": e.repo, "workflow": "name: x\nsteps: []"},
		"unknown agent?": nil,
	} {
		if body == nil {
			continue
		}
		if code, data := e.do("POST", "/api/runs", body); code != 400 {
			t.Errorf("%s: %d %s", name, code, data)
		}
	}
	if code, _ := e.do("POST", "/api/runs", map[string]any{"prompt": "x", "repo": e.repo}, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }); code != 415 {
		t.Errorf("a non-JSON body gave %d, want 415", code)
	}
	if code, _ := e.do("GET", "/api/runs/does-not-exist", nil); code != 404 {
		t.Errorf("unknown run: %d", code)
	}
	if code, _ := e.do("GET", "/api/runs/..%2Fetc", nil); code == 200 {
		t.Errorf("a path-like id was accepted")
	}
}

func TestNoSandboxIsRecordedOnTheWorkflowSteps(t *testing.T) {
	e := newEnv(t, nil)
	e.be.PreflightErr = os.ErrPermission
	code, data := e.do("POST", "/api/runs", map[string]any{"prompt": "x", "repo": e.repo, "base": "main", "agent": "codex", "no_sandbox": true})
	if code != 201 {
		t.Fatalf("%d %s", code, data)
	}
	var created struct{ ID string }
	json.Unmarshal(data, &created)
	if sum := e.waitStatus(created.ID, "succeeded", "failed"); sum["status"] != "succeeded" {
		t.Fatalf("a run that opted out of the sandbox should not be stopped by a broken one: %v", sum["error"])
	}
	if e.be.Preflights != 0 {
		t.Fatal("the sandbox was checked for a run that opted out")
	}
}

func TestCancelAndRetryOverHTTP(t *testing.T) {
	started := make(chan struct{}, 4)
	block := true
	e := newEnv(t, func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		if block {
			started <- struct{}{}
			<-ctx.Done()
			return agent.Result{}, ctx.Err()
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	_, data := e.do("POST", "/api/runs", map[string]any{"prompt": "x", "repo": e.repo, "base": "main", "agent": "codex"})
	var created struct{ ID string }
	json.Unmarshal(data, &created)
	<-started
	if code, d := e.do("POST", "/api/runs/"+created.ID+"/cancel", map[string]any{}); code != 200 {
		t.Fatalf("cancel: %d %s", code, d)
	}
	e.waitStatus(created.ID, "canceled")
	if code, _ := e.do("POST", "/api/runs/"+created.ID+"/cancel", map[string]any{}); code != 409 {
		t.Fatalf("cancelling a finished run gave %d, want 409", code)
	}
	block = false
	if code, d := e.do("POST", "/api/runs/"+created.ID+"/retry", map[string]any{}); code != 200 {
		t.Fatalf("retry: %d %s", code, d)
	}
	if sum := e.waitStatus(created.ID, "succeeded", "failed"); sum["status"] != "succeeded" {
		t.Fatalf("after retry: %v", sum["error"])
	}
	if code, _ := e.do("POST", "/api/runs/"+created.ID+"/retry", map[string]any{}); code != 409 {
		t.Fatalf("retrying a succeeded run gave %d, want 409", code)
	}
}

func TestAccountsOverHTTP(t *testing.T) {
	e := newEnv(t, nil)
	code, data := e.do("POST", "/api/accounts", map[string]any{"id": "second", "provider": "claude", "home": "", "max": 2})
	if code != 201 {
		t.Fatalf("add: %d %s", code, data)
	}
	if _, err := os.Stat(filepath.Join(e.app.Root, "accounts", "claude-second")); err != nil {
		t.Fatalf("the login's folder was not made: %v", err)
	}
	if code, _ := e.do("POST", "/api/accounts", map[string]any{"id": "second", "provider": "claude"}); code != 400 {
		t.Fatalf("duplicate: %d", code)
	}
	if code, _ := e.do("POST", "/api/accounts", map[string]any{"id": "Bad Name", "provider": "claude"}); code != 400 {
		t.Fatalf("bad id: %d", code)
	}
	if code, _ := e.do("POST", "/api/accounts/second/disable", map[string]any{}); code != 200 {
		t.Fatalf("disable: %d", code)
	}
	_, data = e.do("GET", "/api/accounts", nil)
	var accts []pool.Snapshot
	json.Unmarshal(data, &accts)
	if len(accts) != 2 || !accts[1].Disabled {
		t.Fatalf("accounts = %+v", accts)
	}
	if code, _ := e.do("DELETE", "/api/accounts/second", nil); code != 200 {
		t.Fatalf("remove: %d", code)
	}
	if code, _ := e.do("DELETE", "/api/accounts/ghost", nil); code != 404 {
		t.Fatalf("remove ghost: %d", code)
	}
}

func TestARunStreamDeliversEventsAsTheyHappenAndStopsWhenTheClientLeaves(t *testing.T) {
	release := make(chan struct{})
	e := newEnv(t, func(ctx context.Context, req agent.Request, call int, emit func(agent.Event)) (agent.Result, error) {
		emit(agent.Event{Kind: agent.Message, Text: "halfway there"})
		select {
		case <-release:
		case <-ctx.Done():
			return agent.Result{}, ctx.Err()
		}
		return fake.WriteFile(ctx, req, call, emit)
	})
	_, data := e.do("POST", "/api/runs", map[string]any{"prompt": "x", "repo": e.repo, "base": "main", "agent": "codex"})
	var created struct{ ID string }
	json.Unmarshal(data, &created)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.ts.URL+"/api/runs/"+created.ID+"/stream?from=0", nil)
	req.Header.Set("X-Legatus-Token", testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	type frame struct{ event, data string }
	frames := make(chan frame, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
		var ev string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = line[7:]
			case strings.HasPrefix(line, "data: "):
				frames <- frame{ev, line[6:]}
			}
		}
		close(frames)
	}()
	waitFor := func(substr string) {
		t.Helper()
		deadline := time.After(30 * time.Second)
		for {
			select {
			case f, ok := <-frames:
				if !ok {
					t.Fatalf("the stream ended before %q", substr)
				}
				if strings.Contains(f.data, substr) {
					return
				}
			case <-deadline:
				t.Fatalf("never saw %q on the stream", substr)
			}
		}
	}
	waitFor("halfway there")
	close(release)
	waitFor(`"status":"succeeded"`)
	cancel()
	select {
	case _, open := <-frames:
		for open { // drain until the stream closes
			_, open = <-frames
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the stream did not stop after the client left")
	}
}

func TestTheTokenFile(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadOrCreateToken(dir)
	if err != nil || !tokenPattern.MatchString(a) {
		t.Fatalf("token %q, %v", a, err)
	}
	if b, _ := LoadOrCreateToken(dir); b != a {
		t.Fatal("the token changed between loads")
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateToken(dir); err == nil {
		t.Fatal("a damaged token file should be an error, not silently replaced")
	}
}

func TestServeStartsRecordsItselfRefusesASecondAndCleansUp(t *testing.T) {
	e := newEnv(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	bound := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() { done <- e.srv.Serve(ctx, "127.0.0.1:0", func(a net.Addr) { bound <- a }) }()
	var addr net.Addr
	select {
	case addr = <-bound:
	case err := <-done:
		t.Fatalf("Serve ended early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("never became ready")
	}
	d, err := ReadDaemon(e.app.Root)
	if err != nil || d.Addr != addr.String() || d.PID != os.Getpid() {
		t.Fatalf("daemon.json = %+v, %v", d, err)
	}
	if !Ping(d.Addr, testToken) || Ping(d.Addr, strings.Repeat("0", 64)) {
		t.Fatal("Ping should accept the real token and refuse another")
	}
	if err := e.srv.Serve(context.Background(), "127.0.0.1:0", nil); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("a second daemon was allowed: %v", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop")
	}
	if _, err := os.Stat(filepath.Join(e.app.Root, "daemon.json")); !os.IsNotExist(err) {
		t.Fatalf("daemon.json was left behind: %v", err)
	}
}

func TestServeRefusesToListenBeyondThisComputer(t *testing.T) {
	e := newEnv(t, nil)
	for _, addr := range []string{"0.0.0.0:7420", "192.168.1.5:7420", ":7420", "example.com:80"} {
		if err := e.srv.Serve(context.Background(), addr, nil); err == nil || !strings.Contains(err.Error(), "only listens on this computer") {
			t.Errorf("%s: %v", addr, err)
		}
	}
}

type stubIssues struct {
	issue *github.Issue
	err   error
	asked []string
}

func (s *stubIssues) Issue(ctx context.Context, ref, dir string) (*github.Issue, error) {
	s.asked = append(s.asked, ref+"|"+dir)
	return s.issue, s.err
}

type stubPR struct {
	calls []github.PROptions
	mu    sync.Mutex
}

func (s *stubPR) OpenPR(ctx context.Context, o github.PROptions) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, o)
	return "https://github.com/o/r/pull/77", nil
}

func (s *stubPR) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func TestAnIssueBecomesATaskAndItsRunBecomesAPullRequest(t *testing.T) {
	e := newEnv(t, nil)
	issues := &stubIssues{issue: &github.Issue{
		Ref: github.Ref{Owner: "o", Repo: "r", Number: 14}, Title: "Crash on empty input", Body: "It crashes.", Labels: []string{"bug"},
	}}
	prs := &stubPR{}
	e.srv.GitHub = issues
	e.app.Engine.GitHub = prs

	code, data := e.do("POST", "/api/runs", map[string]any{"issue": "o/r#14", "repo": e.repo, "base": "main", "agent": "codex"})
	if code != 201 {
		t.Fatalf("submit: %d %s", code, data)
	}
	var created struct{ ID string }
	json.Unmarshal(data, &created)
	if len(issues.asked) != 1 || !strings.HasPrefix(issues.asked[0], "o/r#14|") {
		t.Fatalf("the issue was asked for as %v", issues.asked)
	}
	e.waitStatus(created.ID, "succeeded")

	_, data = e.do("GET", "/api/runs/"+created.ID, nil)
	var detail struct {
		Run struct {
			Task struct {
				Title, Prompt, Source string
			}
		}
	}
	json.Unmarshal(data, &detail)
	if detail.Run.Task.Title != "Issue #14: Crash on empty input" || detail.Run.Task.Source != "github:o/r#14" ||
		!strings.Contains(detail.Run.Task.Prompt, "<issue>\nIt crashes.\n</issue>") {
		t.Fatalf("task = %+v", detail.Run.Task)
	}

	code, data = e.do("POST", "/api/runs/"+created.ID+"/pr", map[string]any{"draft": true})
	if code != 200 || !strings.Contains(string(data), "pull/77") {
		t.Fatalf("pr: %d %s", code, data)
	}
	if len(prs.calls) != 1 || !strings.HasPrefix(prs.calls[0].Body, "Closes o/r#14") || !prs.calls[0].Draft || prs.calls[0].Base != "main" {
		t.Fatalf("pull request options = %+v", prs.calls)
	}
	_, data = e.do("GET", "/api/runs/"+created.ID, nil)
	if !strings.Contains(string(data), `"pr_url":"https://github.com/o/r/pull/77"`) {
		t.Fatalf("the run does not show its pull request: %s", data)
	}
}

func TestAskingForAPullRequestUpFrontOpensItWhenTheRunSucceeds(t *testing.T) {
	e := newEnv(t, nil)
	prs := &stubPR{}
	e.app.Engine.GitHub = prs
	_, data := e.do("POST", "/api/runs", map[string]any{"prompt": "x", "repo": e.repo, "base": "main", "agent": "codex", "open_pr": "ready"})
	var created struct{ ID string }
	json.Unmarshal(data, &created)
	e.waitStatus(created.ID, "succeeded")
	deadline := time.Now().Add(20 * time.Second)
	for prs.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if prs.count() != 1 || prs.calls[0].Draft {
		t.Fatalf("expected one ready pull request, got %+v", prs.calls)
	}

	// Without the request nothing is pushed or published.
	_, data = e.do("POST", "/api/runs", map[string]any{"prompt": "y", "repo": e.repo, "base": "main", "agent": "codex"})
	var quiet struct{ ID string }
	json.Unmarshal(data, &quiet)
	e.waitStatus(quiet.ID, "succeeded")
	time.Sleep(300 * time.Millisecond)
	if prs.count() != 1 {
		t.Fatal("a run that did not ask for a pull request published one")
	}
}

func TestIssueAndPullRequestRequestsAreValidated(t *testing.T) {
	e := newEnv(t, nil)
	e.srv.GitHub = &stubIssues{err: errors.New("cannot read o/r#9999: Could not resolve to an Issue")}
	for name, body := range map[string]map[string]any{
		"an issue gh cannot read": {"issue": "o/r#9999", "repo": e.repo},
		"a bad pull request mode": {"prompt": "x", "repo": e.repo, "open_pr": "yes"},
		"nothing to do":           {"repo": e.repo},
	} {
		if code, data := e.do("POST", "/api/runs", body); code != 400 {
			t.Errorf("%s: %d %s", name, code, data)
		}
	}
	_, data := e.do("POST", "/api/runs", map[string]any{"issue": "o/r#9999", "repo": e.repo})
	if !strings.Contains(string(data), "Could not resolve") {
		t.Errorf("the reason was not passed on: %s", data)
	}
	if code, _ := e.do("POST", "/api/runs/nope/pr", map[string]any{}); code != 400 {
		t.Errorf("a pull request for an unknown run: %d", code)
	}
}

// The cockpit is plain JavaScript with no build step, so nothing else would catch a typo before a browser did.
func TestTheCockpitScriptParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	data, err := uiFiles.ReadFile("ui/app.js")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", file).CombinedOutput(); err != nil {
		t.Fatalf("app.js does not parse:\n%s", out)
	}
}
