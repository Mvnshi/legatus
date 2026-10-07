package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/runner"
	"github.com/Mvnshi/legatus/internal/pool"
)

// The test binary doubles as a fake `codex`: when LEGATUS_HELPER is set it behaves as the scripted agent.
func TestMain(m *testing.M) {
	if os.Getenv("LEGATUS_HELPER") == "1" {
		helper()
		return
	}
	os.Exit(m.Run())
}

func helper() {
	stdin := new(strings.Builder)
	buf := make([]byte, 64<<10)
	for {
		n, err := os.Stdin.Read(buf)
		stdin.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if rec := os.Getenv("LEGATUS_HELPER_RECORD"); rec != "" {
		data, _ := json.Marshal(map[string]any{"args": os.Args[1:], "stdin": stdin.String(), "codex_home": os.Getenv("CODEX_HOME")})
		os.WriteFile(rec, data, 0o600)
	}
	if last := os.Getenv("LEGATUS_HELPER_LAST"); last != "" {
		for i, a := range os.Args {
			if a == "-o" && i+1 < len(os.Args) {
				os.WriteFile(os.Args[i+1], []byte(last), 0o600)
			}
		}
	}
	if out := os.Getenv("LEGATUS_HELPER_OUT"); out != "" {
		data, _ := os.ReadFile(out)
		os.Stdout.Write(data)
	}
	if e := os.Getenv("LEGATUS_HELPER_ERR"); e != "" {
		os.Stderr.WriteString(e)
	}
	if s := os.Getenv("LEGATUS_HELPER_SLEEP"); s != "" {
		d, _ := time.ParseDuration(s)
		time.Sleep(d)
	}
	code := 0
	if c := os.Getenv("LEGATUS_HELPER_EXIT"); c == "1" {
		code = 1
	}
	os.Exit(code)
}

type script struct {
	out, err, last, sleep string
	exit                  string
}

func (s script) env(t *testing.T, record string) []string {
	t.Helper()
	env := append(os.Environ(), "LEGATUS_HELPER=1", "LEGATUS_HELPER_RECORD="+record,
		"LEGATUS_HELPER_ERR="+s.err, "LEGATUS_HELPER_LAST="+s.last, "LEGATUS_HELPER_SLEEP="+s.sleep, "LEGATUS_HELPER_EXIT="+s.exit)
	if s.out != "" {
		path := filepath.Join(t.TempDir(), "out.jsonl")
		if err := os.WriteFile(path, []byte(s.out), 0o600); err != nil {
			t.Fatal(err)
		}
		env = append(env, "LEGATUS_HELPER_OUT="+path)
	}
	return env
}

func newBackend() *Backend {
	exe, _ := os.Executable()
	return &Backend{Exe: exe, Now: func() time.Time { return time.Date(2026, 10, 7, 14, 0, 0, 0, time.UTC) }}
}

func run(t *testing.T, b *Backend, s script, mutate func(*agent.Request)) (agent.Result, []agent.Event, map[string]any, error) {
	t.Helper()
	record := filepath.Join(t.TempDir(), "record.json")
	req := agent.Request{Dir: t.TempDir(), Prompt: "do the thing", Env: s.env(t, record), Account: pool.Account{ID: "a", Provider: "codex"}}
	if mutate != nil {
		mutate(&req)
	}
	var events []agent.Event
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := b.Run(ctx, req, func(e agent.Event) { events = append(events, e) })
	var rec map[string]any
	if data, rerr := os.ReadFile(record); rerr == nil {
		json.Unmarshal(data, &rec)
	}
	return res, events, rec, err
}

func TestARealCapturedRunParsesToItsMessageAndUsage(t *testing.T) {
	data, err := os.ReadFile("testdata/real-sample.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	res, events, rec, err := run(t, newBackend(), script{out: string(data), last: "OK"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary != "OK" {
		t.Fatalf("summary = %q", res.Summary)
	}
	var sawMessage, sawUsage bool
	for _, e := range events {
		sawMessage = sawMessage || (e.Kind == agent.Message && e.Text == "OK")
		sawUsage = sawUsage || (e.Kind == agent.Usage && e.Data["input_tokens"] != nil)
	}
	if !sawMessage || !sawUsage {
		t.Fatalf("events = %+v", events)
	}
	if rec["stdin"] != "do the thing" {
		t.Fatalf("the prompt did not arrive on stdin: %v", rec["stdin"])
	}
}

func TestTheSummaryFallsBackToTheLastAgentMessage(t *testing.T) {
	out := `{"type":"item.completed","item":{"id":"i","type":"agent_message","text":"first"}}` + "\n" +
		`{"type":"item.completed","item":{"id":"j","type":"agent_message","text":"final words"}}` + "\n" +
		`{"type":"turn.completed","usage":{}}` + "\n"
	res, _, _, err := run(t, newBackend(), script{out: out}, nil)
	if err != nil || res.Summary != "final words" {
		t.Fatalf("summary = %q, err = %v", res.Summary, err)
	}
}

func TestArgumentsFollowTheSandboxAndTheAccount(t *testing.T) {
	b := newBackend()
	b.Model = "gpt-test"
	args := func(rec map[string]any) string {
		var parts []string
		for _, a := range rec["args"].([]any) {
			parts = append(parts, a.(string))
		}
		return strings.Join(parts, " ")
	}
	ok := `{"type":"turn.completed","usage":{}}` + "\n"

	_, _, rec, err := run(t, b, script{out: ok}, func(r *agent.Request) { r.Sandbox = agent.Sandbox{Network: false} })
	if err != nil {
		t.Fatal(err)
	}
	got := args(rec)
	for _, want := range []string{"exec --json --ephemeral", "--ignore-user-config", "--sandbox workspace-write", "sandbox_workspace_write.network_access=false", "-m gpt-test"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if !strings.HasSuffix(got, " -") {
		t.Errorf("the prompt must come from stdin (last argument -): %s", got)
	}

	_, _, rec, _ = run(t, b, script{out: ok}, func(r *agent.Request) { r.Sandbox = agent.Sandbox{Network: true} })
	if !strings.Contains(args(rec), "network_access=true") {
		t.Errorf("network was not enabled: %s", args(rec))
	}
	_, _, rec, _ = run(t, b, script{out: ok}, func(r *agent.Request) { r.Sandbox = agent.Sandbox{ReadOnly: true} })
	got = args(rec)
	if !strings.Contains(got, "--sandbox read-only") || strings.Contains(got, "workspace-write") {
		t.Errorf("a review must be read-only: %s", got)
	}

	env := b.AccountEnv(pool.Account{ID: "a", Provider: "codex", Home: "/pool/a"})
	if env["CODEX_HOME"] != "/pool/a" || b.AccountEnv(pool.Account{ID: "x"}) != nil {
		t.Errorf("AccountEnv = %v", env)
	}
}

func TestAVeryLongPromptSurvivesIntact(t *testing.T) {
	long := strings.Repeat("line of a long diff\n", 20_000) // about 400 KB, far beyond a Windows command line
	_, _, rec, err := run(t, newBackend(), script{out: `{"type":"turn.completed","usage":{}}` + "\n"}, func(r *agent.Request) { r.Prompt = long })
	if err != nil {
		t.Fatal(err)
	}
	if rec["stdin"] != long {
		t.Fatalf("the prompt was changed in transit (%d bytes arrived)", len(rec["stdin"].(string)))
	}
}

func TestAUsageLimitBecomesALimitErrorWithItsResetTime(t *testing.T) {
	out := `{"type":"thread.started","thread_id":"t"}` + "\n" + `{"type":"turn.started"}` + "\n" +
		`{"type":"turn.failed","error":{"message":"You've hit your usage limit. Upgrade to Pro, or try again in 2 hours 30 minutes."}}` + "\n"
	_, _, _, err := run(t, newBackend(), script{out: out, exit: "1"}, nil)
	var le *agent.LimitError
	if !errors.As(err, &le) {
		t.Fatalf("err = %v, want a LimitError", err)
	}
	if want := time.Date(2026, 10, 7, 16, 30, 0, 0, time.UTC); !le.ResetAt.Equal(want) {
		t.Fatalf("ResetAt = %v, want %v", le.ResetAt, want)
	}
}

func TestALimitPrintedOnlyOnStderrIsStillRecognised(t *testing.T) {
	_, _, _, err := run(t, newBackend(), script{err: "ERROR: You've hit your usage limit. try again at 6:15 PM.\n", exit: "1"}, nil)
	var le *agent.LimitError
	if !errors.As(err, &le) || le.ResetAt.Hour() != 18 || le.ResetAt.Minute() != 15 {
		t.Fatalf("err = %v (%+v)", err, le)
	}
}

func TestRecoverableErrorEventsDoNotFailARunThatCompletes(t *testing.T) {
	out := `{"type":"error","message":"Reconnecting... 2/5"}` + "\n" +
		`{"type":"item.completed","item":{"id":"i","type":"agent_message","text":"all good"}}` + "\n" +
		`{"type":"turn.completed","usage":{}}` + "\n"
	res, _, _, err := run(t, newBackend(), script{out: out}, nil)
	if err != nil || res.Summary != "all good" {
		t.Fatalf("summary %q, err %v", res.Summary, err)
	}
}

func TestOtherFailuresCarryTheErrorText(t *testing.T) {
	_, _, _, err := run(t, newBackend(), script{err: "panic: something broke\n", exit: "1"}, nil)
	var le *agent.LimitError
	if err == nil || errors.As(err, &le) || !strings.Contains(err.Error(), "something broke") {
		t.Fatalf("err = %v", err)
	}
}

func TestAMissingProgramSaysSo(t *testing.T) {
	b := &Backend{Exe: "definitely-not-an-installed-agent"}
	_, err := b.Run(context.Background(), agent.Request{Dir: t.TempDir(), Prompt: "x", Env: os.Environ()}, func(agent.Event) {})
	var nf *runner.ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v", err)
	}
}

func TestCancellingStopsTheAgentPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	record := filepath.Join(t.TempDir(), "r.json")
	req := agent.Request{Dir: t.TempDir(), Prompt: "x", Env: script{sleep: "30s"}.env(t, record)}
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := newBackend().Run(ctx, req, func(agent.Event) {})
		done <- err
	}()
	time.Sleep(500 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
		if time.Since(start) > 10*time.Second {
			t.Fatalf("took %s to stop", time.Since(start))
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the agent kept running after cancel")
	}
}

func TestOnWindowsTheLoginsSandboxModeIsCarriedOver(t *testing.T) {
	home := t.TempDir()
	config := "model = \"x\"\n\n[windows]\nsandbox = \"elevated\"\n\n[other]\nsandbox = \"no\"\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := WindowsSandboxMode(home); got != "elevated" {
		t.Fatalf("mode = %q, want elevated", got)
	}
	if got := WindowsSandboxMode(t.TempDir()); got != "unelevated" {
		t.Fatalf("a login that sets nothing should get unelevated, got %q", got)
	}
	otherSection := filepath.Join(t.TempDir(), "x")
	os.MkdirAll(otherSection, 0o700)
	os.WriteFile(filepath.Join(otherSection, "config.toml"), []byte("[other]\nsandbox = \"elevated\"\n"), 0o600)
	if got := WindowsSandboxMode(otherSection); got != "unelevated" {
		t.Fatalf("a sandbox key in another section must not count, got %q", got)
	}

	b := &Backend{GOOS: "windows"}
	args := strings.Join(b.Args(agent.Request{Dir: "d", Account: pool.Account{Home: home}}, "last.txt"), " ")
	if !strings.Contains(args, `windows.sandbox="elevated"`) {
		t.Fatalf("args = %s", args)
	}
	b.GOOS = "linux"
	if args := strings.Join(b.Args(agent.Request{Dir: "d", Account: pool.Account{Home: home}}, "last.txt"), " "); strings.Contains(args, "windows.sandbox") {
		t.Fatalf("the Windows setting must not be passed on other systems: %s", args)
	}
}

func TestUnrestrictedRunsUseNoCodexSandboxAndNoWindowsSetting(t *testing.T) {
	b := &Backend{GOOS: "windows"}
	args := strings.Join(b.Args(agent.Request{Dir: "d", Sandbox: agent.Sandbox{Unrestricted: true}}, "last.txt"), " ")
	if !strings.Contains(args, "--sandbox danger-full-access") || strings.Contains(args, "windows.sandbox") || strings.Contains(args, "workspace-write") {
		t.Fatalf("args = %s", args)
	}
}

func TestPreflightReportsAWorkingAndABrokenWindowsSandbox(t *testing.T) {
	exe, _ := os.Executable()
	win := func() *Backend { return &Backend{Exe: exe, GOOS: "windows"} }
	acct := pool.Account{ID: "a", Provider: "codex", Home: t.TempDir()}
	ctx := context.Background()

	pass := script{out: "legatus-sandbox-ok\n"}
	if err := win().Preflight(ctx, acct, t.TempDir(), pass.env(t, filepath.Join(t.TempDir(), "r.json"))); err != nil {
		t.Fatalf("a working sandbox was reported broken: %v", err)
	}

	denied := script{err: "Access is denied.\n", exit: "1"}
	err := win().Preflight(ctx, acct, t.TempDir(), denied.env(t, filepath.Join(t.TempDir(), "r.json")))
	if err == nil || !strings.Contains(err.Error(), "Access is denied") || !strings.Contains(err.Error(), "cannot start a process") {
		t.Fatalf("err = %v", err)
	}

	// Exit code 0 without the marker is not a pass either.
	silent := script{out: "something else\n"}
	if err := win().Preflight(ctx, acct, t.TempDir(), silent.env(t, filepath.Join(t.TempDir(), "r.json"))); err == nil {
		t.Fatal("a sandbox that printed nothing useful was reported as working")
	}

	// Other systems are not probed.
	if err := (&Backend{Exe: "does-not-exist", GOOS: "linux"}).Preflight(ctx, acct, t.TempDir(), nil); err != nil {
		t.Fatalf("a non-Windows system should not be probed: %v", err)
	}

	// The probe asks for the login's own sandbox mode.
	rec := filepath.Join(t.TempDir(), "r.json")
	os.WriteFile(filepath.Join(acct.Home, "config.toml"), []byte("[windows]\nsandbox = \"elevated\"\n"), 0o600)
	if err := win().Preflight(ctx, acct, t.TempDir(), pass.env(t, rec)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(rec)
	if !strings.Contains(string(data), `windows.sandbox=\"elevated\"`) || !strings.Contains(string(data), `"sandbox"`) {
		t.Fatalf("probe command line = %s", data)
	}
}
