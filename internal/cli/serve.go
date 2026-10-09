package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/app"
	"github.com/Mvnshi/legatus/internal/automation"
	"github.com/Mvnshi/legatus/internal/github"
	"github.com/Mvnshi/legatus/internal/hub"
	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/queue"
	"github.com/Mvnshi/legatus/internal/server"
	"github.com/Mvnshi/legatus/internal/workflow"
)

func cmdServe(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("serve", stderr)
	addr := fs.String("addr", server.DefaultAddr, "where to listen (this computer only)")
	concurrency := fs.Int("concurrency", queue.DefaultConcurrency, "how many runs may work at once")
	checkSlots := fs.Int("check-slots", 1, "how many check commands (builds, test suites) may run at once across all runs")
	root := fs.String("root", "", "where Legatus keeps its files")
	openBrowser := fs.Bool("open", false, "open the cockpit in your browser when it is ready")
	demo := fs.Bool("demo", false, "try the cockpit with stand-in agents and a throwaway repository (nothing real is used)")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	var a *app.App
	var demoRepo string
	if *demo {
		tmp, err := os.MkdirTemp("", "legatus-demo-")
		if err != nil {
			return fail(stderr, err)
		}
		defer os.RemoveAll(tmp)
		if a, err = demoApp(tmp, 1500*time.Millisecond); err != nil {
			return fail(stderr, err)
		}
		demoRepo = filepath.Join(tmp, "repo")
		if err := makeDemoRepo(demoRepo); err != nil {
			return fail(stderr, err)
		}
	} else {
		var ok bool
		if a, ok = openApp(*root, stderr); !ok {
			return 1
		}
	}
	token, err := server.LoadOrCreateToken(a.Root)
	if err != nil {
		return fail(stderr, err)
	}
	h := hub.New()
	a.Engine.CheckSlots = *checkSlots
	sched := &queue.Scheduler{Engine: a.Engine, Hub: h, Concurrency: *concurrency}
	runner := &automation.Runner{
		Dir: a.Root, Submitter: sched, Runs: a.Store, GitHub: &github.Client{},
		Log: func(format string, args ...any) { fmt.Fprintf(stderr, "legatus: "+format+"\n", args...) },
	}
	srv := &server.Server{
		App: a, Sched: sched, Hub: h, Token: token, Version: currentVersion(),
		Runner: runner, AutomationsFile: filepath.Join(a.Root, "automations.yaml"),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	resumed, err := sched.Start(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	defer sched.Stop()
	if *demo {
		sample := fmt.Sprintf(`automations:
  - id: weekly-docs
    schedule: weekly mon 09:00
    repo: %q
    prompt: Refresh the documentation for the export command.
    checks: ["git --version"]
    review: true
    agent: codex
`, filepath.ToSlash(demoRepo))
		if err := os.WriteFile(filepath.Join(a.Root, "automations.yaml"), []byte(sample), 0o600); err != nil {
			return fail(stderr, err)
		}
	}
	go runner.Run(ctx, 30*time.Second)

	err = srv.Serve(ctx, *addr, func(bound net.Addr) {
		fmt.Fprintf(stdout, "Legatus is running at http://%s\n", bound)
		if resumed > 0 {
			fmt.Fprintf(stdout, "Continuing %d run(s) that were not finished.\n", resumed)
		}
		fmt.Fprintln(stdout, "Open the cockpit with:  legatus open")
		fmt.Fprintln(stdout, "Add tasks with:         legatus queue \"what to do\"")
		fmt.Fprintln(stdout, "Stop with Ctrl-C; unfinished runs continue next time.")
		if *demo {
			fmt.Fprintln(stdout, "Demo mode: stand-in agents and a throwaway repository. The first login runs out of usage, as it would for real.")
			fmt.Fprintln(stdout, "Open this (its key is only good for this demo):", cockpitURL(bound.String(), token))
			go func() {
				tasks := []struct {
					prompt string
					review bool
				}{
					{"Add rate limiting to the login endpoint", true},
					{"Fix the flaky invoice export test", false},
					{"Write documentation for the export command", true},
					{"Upgrade the date library and fix what breaks", false},
				}
				for _, t := range tasks {
					wf := workflow.Default([]string{"git --version"}, t.review)
					wf.Steps[0].Agent = "codex" // the stand-in Claude login only reviews
					if _, err := sched.Submit(ctx, model.Task{Prompt: t.prompt, Repo: demoRepo, Base: "main"}, wf); err != nil {
						fmt.Fprintln(stderr, "legatus: demo task:", err)
					}
					if pause(ctx, 1200*time.Millisecond) != nil {
						return
					}
				}
			}()
		}
		if *openBrowser {
			if err := openURL(cockpitURL(bound.String(), token)); err != nil {
				fmt.Fprintln(stderr, "legatus: could not open a browser:", err)
			}
		}
	})
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, "Stopped.")
	return 0
}

// cockpitURL puts the key in the fragment, which a browser never sends to any server or writes to a log.
func cockpitURL(addr, token string) string { return "http://" + addr + "/#token=" + token }

func openURL(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// daemon finds the running daemon for these files and its key.
func daemon(rootFlag string, stderr io.Writer) (addr, token string, ok bool) {
	a, ok := openApp(rootFlag, stderr)
	if !ok {
		return "", "", false
	}
	token, err := server.LoadOrCreateToken(a.Root)
	if err != nil {
		fmt.Fprintln(stderr, "legatus:", err)
		return "", "", false
	}
	d, err := server.ReadDaemon(a.Root)
	if err != nil || !server.Ping(d.Addr, token) {
		fmt.Fprintln(stderr, "legatus: the daemon is not running. Start it in another terminal with:\n\n  legatus serve")
		return "", "", false
	}
	return d.Addr, token, true
}

func cmdOpen(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("open", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	printOnly := fs.Bool("print", false, "print the address (with its key) instead of opening a browser")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	addr, token, ok := daemon(*root, stderr)
	if !ok {
		return 1
	}
	url := cockpitURL(addr, token)
	if *printOnly {
		fmt.Fprintln(stdout, url)
		return 0
	}
	if err := openURL(url); err != nil {
		fmt.Fprintf(stderr, "legatus: could not open a browser (%v). Open http://%s and use `legatus open --print` for the key.\n", err, addr)
		return 1
	}
	fmt.Fprintf(stdout, "Opened the cockpit at http://%s\n", addr)
	return 0
}

// cmdQueue sends a task to the running daemon instead of running it in this terminal.
func cmdQueue(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("queue", stderr)
	repo := fs.String("repo", ".", "the git repository to work in")
	base := fs.String("base", "HEAD", "the branch or commit to start from")
	wfPath := fs.String("workflow", "", "a workflow file")
	var checks stringList
	fs.Var(&checks, "check", "a command that must pass (repeatable)")
	review := fs.Bool("review", false, "have an independent agent review the change")
	agentFlag := fs.String("agent", "any", "which agent: any, codex, claude")
	noSandbox := fs.Bool("no-sandbox", false, "run agents without their own sandbox")
	issueRef := fs.String("issue", "", "work on a GitHub issue: owner/repo#12, its URL, or a number")
	prMode := fs.String("pr", "", "when the run succeeds, push its branch and open a pull request: draft or ready")
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" && *issueRef == "" {
		fmt.Fprintln(stderr, "legatus: say what to do, e.g. legatus queue \"fix the failing login test\", or give --issue")
		return 64
	}
	if *prMode != "" && *prMode != "draft" && *prMode != "ready" {
		fmt.Fprintln(stderr, "legatus: --pr must be draft or ready")
		return 64
	}
	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		return fail(stderr, err)
	}
	body := map[string]any{
		"prompt": prompt, "repo": absRepo, "base": *base, "checks": []string(checks), "review": *review,
		"agent": *agentFlag, "no_sandbox": *noSandbox, "issue": *issueRef, "open_pr": *prMode,
	}
	if *wfPath != "" {
		data, err := os.ReadFile(*wfPath)
		if err != nil {
			return fail(stderr, err)
		}
		body["workflow"] = string(data)
	}
	addr, token, ok := daemon(*root, stderr)
	if !ok {
		return 1
	}
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "http://"+addr+"/api/runs", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Legatus-Token", token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fail(stderr, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fail(stderr, fmt.Errorf("%s", e.Error))
	}
	var run struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(data, &run)
	fmt.Fprintf(stdout, "Queued run %s. Watch it in the cockpit (legatus open) or with: legatus show -events %s\n", run.ID, run.ID)
	return 0
}
