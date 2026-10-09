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

// The test binary doubles as a fake `claude`, as in the codex tests. Most scripted output is written by hand
// in Claude Code's stream-json format; testdata/real-*.jsonl is the output of real runs of Claude Code
// 2.1.295 (see TestRealRuns...), and the limit cases say how they were built.
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

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// real-reply.jsonl and real-sample.jsonl are the stdout of Claude Code 2.1.295 on Linux, run the way Legatus
// runs it (`claude -p --output-format stream-json --verbose`, the prompt on stdin). Names, ids, paths and
// the long lists in the init event were replaced; the structure and every event type are as printed.
func TestRealRunsOfClaudeCodeParse(t *testing.T) {
	res, events, _, err := run(t, fixture(t, "real-reply.jsonl"), "", "", nil)
	if err != nil || res.Summary != "OK" {
		t.Fatalf("a real plain reply: summary %q, err %v", res.Summary, err)
	}
	if len(events) == 0 {
		t.Fatal("a real run produced no events")
	}

	res, events, _, err = run(t, fixture(t, "real-sample.jsonl"), "", "", nil)
	if err != nil || res.Summary != "It printed `ok`." {
		t.Fatalf("a real run that used a tool: summary %q, err %v", res.Summary, err)
	}
	var tools []string
	usage := 0
	for _, e := range events {
		switch e.Kind {
		case agent.Tool:
			tools = append(tools, e.Text)
		case agent.Usage:
			usage++
		}
	}
	if len(tools) != 1 || tools[0] != "Bash" || usage != 1 {
		t.Fatalf("tool events %v, usage events %d", tools, usage)
	}
}

// A real run reports its usage windows as "allowed". That must never read as a limit.
func TestARealAllowedRateLimitEventIsNotALimit(t *testing.T) {
	_, _, _, err := run(t, fixture(t, "real-sample.jsonl"), "", "", nil)
	var le *agent.LimitError
	if errors.As(err, &le) {
		t.Fatalf("an allowed rate_limit_event became a limit: %+v", le)
	}
}

// Built, not captured: the real rate_limit_event of the fixtures with its status changed to "rejected" and
// the failure text taken from the template Claude Code 2.1.295 uses. A real refusal was not provoked.
const rejectedStream = `{"type":"system","subtype":"init","session_id":"s"}
{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":1791573600,"rateLimitType":"five_hour","overageStatus":"rejected","overageResetsAt":1791907200,"overageDisabledReason":"out_of_credits","isUsingOverage":false}}
{"type":"result","subtype":"success","is_error":true,"result":"You've hit your session limit · resets 3pm (America/New_York)"}
`

func TestARejectedRateLimitEventGivesTheResetAsATimestamp(t *testing.T) {
	_, _, _, err := run(t, rejectedStream, "", "1", nil)
	var le *agent.LimitError
	if !errors.As(err, &le) || le.ResetAt.Unix() != 1791573600 {
		t.Fatalf("err = %v (%+v)", err, le)
	}
}

func TestTheEarlierOfTheWindowAndOverageResetWins(t *testing.T) {
	out := strings.Replace(rejectedStream, `"resetsAt":1791573600`, `"resetsAt":1791999999`, 1)
	_, _, _, err := run(t, out, "", "1", nil)
	var le *agent.LimitError
	if !errors.As(err, &le) || le.ResetAt.Unix() != 1791907200 {
		t.Fatalf("err = %v (%+v)", err, le)
	}
}

func TestARejectedEventWithoutAFailureIsNotALimit(t *testing.T) {
	// The overage can carry a run past a rejected window; if the run succeeded, it succeeded.
	out := `{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":1791573600,"rateLimitType":"five_hour"}}` + "\n" + okStream
	res, _, _, err := run(t, out, "", "", nil)
	if err != nil || res.Summary != "Changed a.go." {
		t.Fatalf("summary %q, err %v", res.Summary, err)
	}
}

func TestTheNamedZoneInTheMessageIsHonouredWithoutAnEvent(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	out := `{"type":"result","subtype":"success","is_error":true,"result":"You've hit your session limit · resets 3pm (America/New_York)"}` + "\n"
	_, _, _, err := run(t, out, "", "1", nil)
	var le *agent.LimitError
	// The backend's clock in these tests is 14:00 UTC on 7 October 2026, so 3pm in New York is still ahead.
	want := time.Date(2026, 10, 7, 15, 0, 0, 0, ny)
	if !errors.As(err, &le) || !le.ResetAt.Equal(want) {
		t.Fatalf("err = %v, reset %v, want %v", err, le, want)
	}
}

func TestAServerThrottleIsAnErrorNotALimit(t *testing.T) {
	out := `{"type":"result","subtype":"success","is_error":true,"result":"API Error: Server is temporarily limiting requests (not your usage limit)"}` + "\n"
	_, _, _, err := run(t, out, "", "1", nil)
	var le *agent.LimitError
	if err == nil || errors.As(err, &le) {
		t.Fatalf("a throttle that is not the login's limit must fail the call, not set the login aside: %v", err)
	}
}

// real-api-429.jsonl is the stdout of Claude Code 2.1.295 (exit code 1) when the API refused its one request
// with HTTP 429 and the unified rate-limit headers (status rejected, five_hour) on an API-key login. Only the
// server was simulated, by a local stand-in; what the CLI printed is real. This login type prints no
// rate_limit_event and no "You've hit your ... limit" line: just "API Error: Request rejected (429) · <message>".
func TestARealRefusalFromTheAPIIsALimitError(t *testing.T) {
	_, _, _, err := run(t, fixture(t, "real-api-429.jsonl"), "", "1", nil)
	var le *agent.LimitError
	if !errors.As(err, &le) {
		t.Fatalf("a real 429 refusal must set the login aside, not fail the task: %v", err)
	}
}

// The same real refusal with the server's message changed to words that name no limit at all. The structured
// status (429) is what marks it as the login being refused.
func TestARefusalIsRecognisedByItsStatusNotOnlyItsWords(t *testing.T) {
	out := strings.ReplaceAll(fixture(t, "real-api-429.jsonl"), "usage limit", "slow down")
	if out == fixture(t, "real-api-429.jsonl") {
		t.Fatal("the fixture no longer contains the words this test replaces")
	}
	_, _, _, err := run(t, out, "", "1", nil)
	var le *agent.LimitError
	if !errors.As(err, &le) {
		t.Fatalf("err = %v", err)
	}
	// ...unless the service says it is throttling everyone.
	out = strings.ReplaceAll(fixture(t, "real-api-429.jsonl"), "usage limit", "Server is temporarily limiting requests (not your usage limit)")
	_, _, _, err = run(t, out, "", "1", nil)
	if err == nil || errors.As(err, &le) {
		t.Fatalf("a throttle of the whole service must not set the login aside: %v", err)
	}
}

// real-session-limit.jsonl is the stdout of Claude Code 2.1.295 (exit code 1) when its one request was refused
// with HTTP 429 and the unified rate-limit headers (status rejected, five_hour, overage out_of_credits). The
// refusal was simulated by a local stand-in for the API, because a real login cannot be run out of usage on
// demand; everything the CLI printed in answer is real. The reset it prints in words ("resets 8:54pm (UTC)")
// is the minute-rounded reading of the rate_limit_event's exact time, 1791579283.
func TestARealSessionLimitGivesTheExactResetFromItsEvent(t *testing.T) {
	_, events, _, err := run(t, fixture(t, "real-session-limit.jsonl"), "", "1", nil)
	var le *agent.LimitError
	if !errors.As(err, &le) {
		t.Fatalf("err = %v", err)
	}
	if le.ResetAt.Unix() != 1791579283 {
		t.Fatalf("reset = %v (%d), want the event's 1791579283", le.ResetAt, le.ResetAt.Unix())
	}
	if !strings.Contains(le.Message, "You've hit your session limit") {
		t.Fatalf("message = %q", le.Message)
	}
	said := false
	for _, e := range events {
		said = said || (e.Kind == agent.Message && strings.Contains(e.Text, "session limit"))
	}
	if !said {
		t.Fatal("the limit message the agent printed was not shown in the run's journal")
	}
}

// Without the event, the words alone still give the login's reset, to the minute.
func TestARealSessionLimitMessageAloneStillGivesItsReset(t *testing.T) {
	var kept []string
	for _, line := range strings.Split(fixture(t, "real-session-limit.jsonl"), "\n") {
		if !strings.Contains(line, `"type": "rate_limit_event"`) && !strings.Contains(line, `"type":"rate_limit_event"`) {
			kept = append(kept, line)
		}
	}
	_, _, _, err := run(t, strings.Join(kept, "\n"), "", "1", nil)
	var le *agent.LimitError
	// The backend's clock in these tests is 14:00 UTC on 7 October 2026: the next 8:54pm UTC is that evening.
	want := time.Date(2026, 10, 7, 20, 54, 0, 0, time.UTC)
	if !errors.As(err, &le) || !le.ResetAt.Equal(want) {
		t.Fatalf("err = %v, reset %v, want %v", err, le, want)
	}
}
