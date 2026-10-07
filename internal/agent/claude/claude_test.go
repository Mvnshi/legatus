package claude

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
	"github.com/Mvnshi/legatus/internal/pool"
)

// The test binary doubles as a fake `claude`, as in the codex tests. The scripted output is written from
// Claude Code's documented stream-json format, not captured from a real install.
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
		data, _ := json.Marshal(map[string]any{"args": os.Args[1:], "stdin": stdin.String(), "config_dir": os.Getenv("CLAUDE_CONFIG_DIR")})
		os.WriteFile(rec, data, 0o600)
	}
	if out := os.Getenv("LEGATUS_HELPER_OUT"); out != "" {
		data, _ := os.ReadFile(out)
		os.Stdout.Write(data)
	}
	if e := os.Getenv("LEGATUS_HELPER_ERR"); e != "" {
		os.Stderr.WriteString(e)
	}
	if os.Getenv("LEGATUS_HELPER_EXIT") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

func run(t *testing.T, stdout, stderr string, exit string, mutate func(*agent.Request)) (agent.Result, []agent.Event, map[string]any, error) {
	t.Helper()
	exe, _ := os.Executable()
	b := &Backend{Exe: exe, Now: func() time.Time { return time.Date(2026, 10, 7, 14, 0, 0, 0, time.UTC) }}
	record := filepath.Join(t.TempDir(), "rec.json")
	env := append(os.Environ(), "LEGATUS_HELPER=1", "LEGATUS_HELPER_RECORD="+record, "LEGATUS_HELPER_ERR="+stderr, "LEGATUS_HELPER_EXIT="+exit)
	if stdout != "" {
		p := filepath.Join(t.TempDir(), "out.jsonl")
		os.WriteFile(p, []byte(stdout), 0o600)
		env = append(env, "LEGATUS_HELPER_OUT="+p)
	}
	req := agent.Request{Dir: t.TempDir(), Prompt: "do it", Env: env, Account: pool.Account{ID: "c1", Provider: "claude"}}
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

const okStream = `{"type":"system","subtype":"init","session_id":"s"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Looking."},{"type":"tool_use","name":"Read","input":{"file_path":"a.go"}}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Done."}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Changed a.go.","total_cost_usd":0.012,"usage":{"input_tokens":10,"output_tokens":5}}
`

func TestAScriptedRunParsesToItsResult(t *testing.T) {
	res, events, rec, err := run(t, okStream, "", "", nil)
	if err != nil || res.Summary != "Changed a.go." {
		t.Fatalf("summary %q, err %v", res.Summary, err)
	}
	kinds := map[agent.EventKind]int{}
	for _, e := range events {
		kinds[e.Kind]++
	}
	if kinds[agent.Message] != 2 || kinds[agent.Tool] != 1 || kinds[agent.Usage] != 1 {
		t.Fatalf("events = %+v", kinds)
	}
	if rec["stdin"] != "do it" {
		t.Fatalf("prompt on stdin = %v", rec["stdin"])
	}
}

func TestArgumentsFollowTheSandboxAndTheAccount(t *testing.T) {
	join := func(rec map[string]any) string {
		var parts []string
		for _, a := range rec["args"].([]any) {
			parts = append(parts, a.(string))
		}
		return strings.Join(parts, " ")
	}
	_, _, rec, _ := run(t, okStream, "", "", nil)
	got := join(rec)
	for _, want := range []string{"-p --output-format stream-json --verbose", "--permission-mode acceptEdits", "Edit,Write,Bash", "--disallowedTools WebFetch,WebSearch"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	_, _, rec, _ = run(t, okStream, "", "", func(r *agent.Request) { r.Sandbox = agent.Sandbox{ReadOnly: true} })
	got = join(rec)
	if !strings.Contains(got, "--allowedTools Read,Grep,Glob") || !strings.Contains(got, "--disallowedTools Edit,Write,Bash") || strings.Contains(got, "acceptEdits") {
		t.Errorf("a review must not be able to edit or run commands: %s", got)
	}
	_, _, rec, _ = run(t, okStream, "", "", func(r *agent.Request) { r.Sandbox = agent.Sandbox{Network: true} })
	if strings.Contains(join(rec), "disallowedTools") {
		t.Errorf("network was asked for but the web tools are still blocked: %s", join(rec))
	}
	b := &Backend{}
	if env := b.AccountEnv(pool.Account{Home: "/pool/c"}); env["CLAUDE_CONFIG_DIR"] != "/pool/c" || b.AccountEnv(pool.Account{}) != nil {
		t.Errorf("AccountEnv = %v", env)
	}
}

func TestALimitInTheResultBecomesALimitErrorWithItsReset(t *testing.T) {
	out := `{"type":"result","subtype":"success","is_error":true,"result":"5-hour limit reached ∙ resets 7pm"}` + "\n"
	_, _, _, err := run(t, out, "", "1", nil)
	var le *agent.LimitError
	if !errors.As(err, &le) || le.ResetAt.Hour() != 19 {
		t.Fatalf("err = %v (%+v)", err, le)
	}
}

func TestALimitOnStderrIsRecognisedToo(t *testing.T) {
	_, _, _, err := run(t, "", "Claude AI usage limit reached|1791500000\n", "1", nil)
	var le *agent.LimitError
	if !errors.As(err, &le) || le.ResetAt.Unix() != 1791500000 {
		t.Fatalf("err = %v (%+v)", err, le)
	}
}

func TestOtherErrorsAndAMissingResultFail(t *testing.T) {
	out := `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"tool crashed"}` + "\n"
	_, _, _, err := run(t, out, "", "1", nil)
	var le *agent.LimitError
	if err == nil || errors.As(err, &le) || !strings.Contains(err.Error(), "tool crashed") {
		t.Fatalf("err = %v", err)
	}
	_, _, _, err = run(t, "", "", "", nil)
	if err == nil || !strings.Contains(err.Error(), "without a result") {
		t.Fatalf("a run that printed no result must not count as success: %v", err)
	}
}
