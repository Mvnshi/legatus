// Package server is the daemon's HTTP side: a JSON API, live streams and the embedded web cockpit.
//
// It is meant for one person on one machine. It listens on the loopback address, rejects requests whose
// Host is not a loopback name (so a web page cannot reach it through DNS rebinding), and requires a secret
// token on every API call. Anyone holding the token can have agents run in any repository on this machine,
// so the token is a file only the user can read, and it is never put in a URL that is logged (the cockpit
// receives it in the URL fragment, which browsers do not send).
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/app"
	"github.com/Mvnshi/legatus/internal/github"
	"github.com/Mvnshi/legatus/internal/hub"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/queue"
	"github.com/Mvnshi/legatus/internal/store"
	"github.com/Mvnshi/legatus/internal/workflow"
)

// Server serves one Legatus installation.
type Server struct {
	App     *app.App
	Sched   *queue.Scheduler
	Hub     *hub.Hub
	Token   string
	Version string
	// GitHub reads issues; the real gh-based client when nil.
	GitHub IssueReader
}

// IssueReader reads a GitHub issue. *github.Client is the real one.
type IssueReader interface {
	Issue(ctx context.Context, ref, dir string) (*github.Issue, error)
}

const maxBody = 1 << 20

// Handler returns the whole site: API under /api, the cockpit everywhere else.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, s.guard(h)) }

	api("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{"ok": true}) })
	api("GET /api/state", s.handleState)
	api("GET /api/stream", s.handleStream)
	api("GET /api/runs", s.handleRuns)
	api("POST /api/runs", s.handleSubmit)
	api("GET /api/runs/{id}", s.handleRun)
	api("GET /api/runs/{id}/stream", s.handleRunStream)
	api("GET /api/runs/{id}/diff", s.handleDiff)
	api("POST /api/runs/{id}/cancel", s.handleCancel)
	api("POST /api/runs/{id}/retry", s.handleRetry)
	api("POST /api/runs/{id}/pr", s.handlePR)
	api("GET /api/accounts", s.handleAccounts)
	api("POST /api/accounts", s.handleAddAccount)
	api("POST /api/accounts/{id}/enable", s.handleToggleAccount(false))
	api("POST /api/accounts/{id}/disable", s.handleToggleAccount(true))
	api("DELETE /api/accounts/{id}", s.handleRemoveAccount)
	mux.Handle("/", s.ui())
	return mux
}

// guard rejects requests that are not from the user's own machine and browser session.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			writeErr(w, http.StatusForbidden, "this server only answers to localhost")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !sameHost(origin, r.Host) {
			writeErr(w, http.StatusForbidden, "cross-origin requests are not allowed")
			return
		}
		got := r.Header.Get("X-Legatus-Token")
		if s.Token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) != 1 {
			writeErr(w, http.StatusUnauthorized, "missing or wrong token")
			return
		}
		next(w, r)
	}
}

func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sameHost(origin, hostport string) bool {
	rest := origin
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	return rest == hostport
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeErr(w, http.StatusUnsupportedMediaType, "send application/json")
		return false
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "could not read the request: "+err.Error())
		return false
	}
	return true
}

// --- state and lists -------------------------------------------------------------------------------------

type stepSummary struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Account  string `json:"account,omitempty"`
	Attempts int    `json:"attempts"`
	Summary  string `json:"summary,omitempty"`
}

type runSummary struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Status    string        `json:"status"`
	Repo      string        `json:"repo"`
	Branch    string        `json:"branch"`
	Error     string        `json:"error,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	WaitUntil *time.Time    `json:"wait_until,omitempty"`
	PRURL     string        `json:"pr_url,omitempty"`
	Source    string        `json:"source,omitempty"`
	Current   int           `json:"current"`
	Steps     []stepSummary `json:"steps"`
}

func summarize(r *model.Run) runSummary {
	out := runSummary{
		ID: r.ID, Title: r.Task.Title, Status: string(r.Status), Repo: r.Task.Repo, Branch: r.Branch, Error: r.Error,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Current: r.Current, PRURL: r.PRURL, Source: r.Task.Source,
	}
	if !r.WaitUntil.IsZero() {
		t := r.WaitUntil
		out.WaitUntil = &t
	}
	for _, st := range r.Steps {
		out.Steps = append(out.Steps, stepSummary{
			ID: st.ID, Kind: string(st.Kind), Status: string(st.Status), Account: st.Account, Attempts: st.Attempts, Summary: st.Summary,
		})
	}
	return out
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	runs, err := s.App.Store.List()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	var repos []string
	for _, run := range runs {
		counts[string(run.Status)]++
		if run.Task.Repo != "" && !seen[run.Task.Repo] {
			seen[run.Task.Repo] = true
			repos = append(repos, run.Task.Repo)
		}
	}
	if len(repos) > 8 {
		repos = repos[:8]
	}
	writeJSON(w, 200, map[string]any{
		"version":      s.Version,
		"now":          time.Now().UTC(),
		"queue":        s.Sched.Stats(),
		"counts":       counts,
		"accounts":     s.App.Pool.Snapshots(),
		"recent_repos": repos,
	})
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.App.Store.List()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	limit := 200
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n < limit {
		limit = n
	}
	status := r.URL.Query().Get("status")
	out := []runSummary{}
	for _, run := range runs {
		if status != "" && string(run.Status) != status {
			continue
		}
		out = append(out, summarize(run))
		if len(out) >= limit {
			break
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) loadRun(w http.ResponseWriter, r *http.Request) (*model.Run, bool) {
	run, err := s.App.Store.Load(r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "no such run")
		return nil, false
	case err != nil:
		writeErr(w, 400, err.Error())
		return nil, false
	}
	return run, true
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	run, ok := s.loadRun(w, r)
	if !ok {
		return
	}
	evs, next, _ := s.App.Store.Events(run.ID, 0)
	report := ""
	if dir, err := s.App.Store.RunDir(run.ID); err == nil {
		report = readText(dir + "/evidence.md")
	}
	writeJSON(w, 200, map[string]any{
		"run": run, "summary": summarize(run), "events": evs, "next": next, "evidence": report,
	})
}

// --- submitting and controlling runs ------------------------------------------------------------------------

type submitRequest struct {
	Prompt    string   `json:"prompt"`
	Title     string   `json:"title"`
	Repo      string   `json:"repo"`
	Base      string   `json:"base"`
	Checks    []string `json:"checks"`
	Review    bool     `json:"review"`
	Agent     string   `json:"agent"`
	NoSandbox bool     `json:"no_sandbox"`
	Workflow  string   `json:"workflow"`
	Issue     string   `json:"issue"`   // a GitHub issue to work on: owner/repo#12, its URL, or a number
	OpenPR    string   `json:"open_pr"` // "", "draft" or "ready": open a pull request when the run succeeds
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req submitRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Repo) == "" {
		writeErr(w, 400, "choose a repository")
		return
	}
	if req.OpenPR != "" && req.OpenPR != "draft" && req.OpenPR != "ready" {
		writeErr(w, 400, "open_pr must be draft or ready")
		return
	}
	source := ""
	if strings.TrimSpace(req.Issue) != "" {
		var reader IssueReader = s.GitHub
		if reader == nil {
			reader = &github.Client{}
		}
		issue, err := reader.Issue(r.Context(), req.Issue, req.Repo)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		source = "github:" + issue.Ref.String()
		if strings.TrimSpace(req.Title) == "" {
			req.Title = fmt.Sprintf("Issue #%d: %s", issue.Ref.Number, issue.Title)
		}
		if strings.TrimSpace(req.Prompt) != "" {
			req.Prompt = strings.TrimSpace(req.Prompt) + "\n\n" + github.TaskPrompt(issue)
		} else {
			req.Prompt = github.TaskPrompt(issue)
		}
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeErr(w, 400, "say what to do, or give an issue")
		return
	}
	var wf *workflow.Workflow
	if strings.TrimSpace(req.Workflow) != "" {
		var err error
		if wf, err = workflow.Parse([]byte(req.Workflow)); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	} else {
		var checks []string
		for _, c := range req.Checks {
			if strings.TrimSpace(c) != "" {
				checks = append(checks, strings.TrimSpace(c))
			}
		}
		wf = workflow.Default(checks, req.Review)
		agentName := req.Agent
		if agentName == "" {
			agentName = "any"
		}
		wf.Steps[0].Agent = agentName
		if err := wf.Validate(); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if req.NoSandbox {
		for i := range wf.Steps {
			if wf.Steps[i].Kind() != model.StepCheck {
				wf.Steps[i].Sandbox.Mode = "none"
			}
		}
	}
	run, err := s.Sched.Submit(r.Context(), model.Task{
		Prompt: req.Prompt, Title: req.Title, Repo: req.Repo, Base: req.Base, Source: source, OpenPR: req.OpenPR,
	}, wf)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, summarize(run))
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.Sched.Cancel(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleRetry(w http.ResponseWriter, r *http.Request) {
	if err := s.Sched.Retry(r.PathValue("id")); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handlePR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Draft bool `json:"draft"`
	}
	if !decode(w, r, &req) {
		return
	}
	url, err := s.App.Engine.OpenPullRequest(r.Context(), r.PathValue("id"), req.Draft)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.Hub.Notify(r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"url": url})
}

const maxDiff = 400 << 10

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	run, ok := s.loadRun(w, r)
	if !ok {
		return
	}
	patch, err := s.App.Engine.Worktrees.Patch(r.Context(), run.Worktree, run.BaseCommit, maxDiff)
	if err != nil {
		writeErr(w, 404, "there is no worktree to compare (it has not started, or was cleaned)")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, patch)
}

// --- accounts --------------------------------------------------------------------------------------------

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.App.Pool.Snapshots())
}

type addAccountRequest struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Home     string `json:"home"`
	Label    string `json:"label"`
	Max      int    `json:"max"`
}

func (s *Server) handleAddAccount(w http.ResponseWriter, r *http.Request) {
	var req addAccountRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Max == 0 {
		req.Max = 1
	}
	acct, err := s.App.AddAccount(req.ID, req.Provider, req.Home, req.Label, req.Max)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.Hub.Notify("")
	writeJSON(w, http.StatusCreated, acct)
}

func (s *Server) handleToggleAccount(disable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.App.SetAccountDisabled(r.PathValue("id"), disable); err != nil {
			writeErr(w, 404, err.Error())
			return
		}
		s.Hub.Notify("")
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func (s *Server) handleRemoveAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.App.RemoveAccount(r.PathValue("id")); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	s.Hub.Notify("")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- live streams ----------------------------------------------------------------------------------------

type sse struct {
	w http.ResponseWriter
	f http.Flusher
}

func newSSE(w http.ResponseWriter) (*sse, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming is not supported here")
		return nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	f.Flush()
	return &sse{w: w, f: f}, true
}

func (e *sse) send(event string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	e.f.Flush()
	return nil
}

func (e *sse) ping() error {
	if _, err := io.WriteString(e.w, ": ping\n\n"); err != nil {
		return err
	}
	e.f.Flush()
	return nil
}

// handleStream tells the cockpit that something changed, so it can refetch what it shows.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	out, ok := newSSE(w)
	if !ok {
		return
	}
	_ = out.send("hello", map[string]bool{"ok": true})
	for {
		ch := s.Hub.Changed()
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if out.send("change", map[string]bool{"changed": true}) != nil {
				return
			}
		case <-time.After(15 * time.Second):
			if out.ping() != nil {
				return
			}
		}
	}
}

// handleRunStream follows one run: new journal events, and the run's state whenever it changes.
func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	run, ok := s.loadRun(w, r)
	if !ok {
		return
	}
	from, _ := strconv.Atoi(r.URL.Query().Get("from"))
	if from < 0 {
		from = 0
	}
	out, ok := newSSE(w)
	if !ok {
		return
	}
	id := run.ID
	for {
		ch := s.Hub.RunChanged(id) // taken before reading, so a change in between is not missed
		if cur, err := s.App.Store.Load(id); err == nil {
			if out.send("run", summarize(cur)) != nil {
				return
			}
		}
		evs, next, err := s.App.Store.Events(id, from)
		if err == nil && len(evs) > 0 {
			if out.send("events", map[string]any{"events": evs, "next": next}) != nil {
				return
			}
			from = next
		}
		select {
		case <-r.Context().Done():
			return
		case <-ch:
		case <-time.After(15 * time.Second):
			if out.ping() != nil {
				return
			}
		}
	}
}

// --- helpers ---------------------------------------------------------------------------------------------

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
