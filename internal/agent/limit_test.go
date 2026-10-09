package agent

import (
	"fmt"
	"testing"
	"time"
)

func TestIsLimitMessage(t *testing.T) {
	for _, s := range []string{
		"You've hit your usage limit. Upgrade to Pro, or try again at 3:41 PM.",
		"Claude AI usage limit reached|1790000000",
		"5-hour limit reached ∙ resets 7pm",
		`{"error":{"type":"usage_limit_reached"}}`,
		"429 Too Many Requests",
		"You exceeded your current quota",
	} {
		if !IsLimitMessage(s) {
			t.Errorf("not recognised: %q", s)
		}
	}
	for _, s := range []string{"segmentation fault", "file not found: usage.txt", "network unreachable", ""} {
		if IsLimitMessage(s) {
			t.Errorf("wrongly recognised: %q", s)
		}
	}
}

// These are the messages Claude Code 2.1.295 builds (read from its own program text, see docs/STATUS.md),
// with a made-up time. The first form is "You've hit your <name of the limit> · resets <time> (<zone>)".
func TestIsLimitMessageKnowsWhatClaudeCodePrints(t *testing.T) {
	for _, s := range []string{
		"You've hit your session limit · resets 3pm (America/New_York)",
		"You've hit your weekly limit · resets Oct 12, 3pm (America/New_York)",
		"You've hit your Opus limit · resets 9:30am (Europe/London)",
		"You've hit your Sonnet limit · resets 3pm (UTC)",
		"You've hit your org's monthly spend limit · ask your admin to raise it",
		"You've hit your team's shared budget. /model to switch models.",
		"You're out of usage credits. Run /usage-credits to keep using it",
		"Your org is out of usage · add funds to continue",
		"Your credit balance is too low to access the Anthropic API. Please go to Plans & Billing.",
	} {
		if !IsLimitMessage(s) {
			t.Errorf("a login limit that Claude Code prints was not recognised: %q", s)
		}
	}
}

// "Server is temporarily limiting requests (not your usage limit)" means the service is throttling
// everyone. Setting a working login aside for hours because of it would be wrong.
func TestAServerThrottleIsNotALoginLimit(t *testing.T) {
	for _, s := range []string{
		"Server is temporarily limiting requests (not your usage limit)",
		"API Error: Server is temporarily limiting requests (not your usage limit) · retrying",
	} {
		if IsLimitMessage(s) {
			t.Errorf("a server throttle was taken for a login limit: %q", s)
		}
	}
}

func TestParseReset(t *testing.T) {
	// Wednesday 7 October 2026, 14:00 in a zone that is not UTC.
	zone := time.FixedZone("EDT", -4*3600)
	now := time.Date(2026, 10, 7, 14, 0, 0, 0, zone)
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, zone) }

	cases := []struct {
		msg  string
		want time.Time
	}{
		{"You've hit your usage limit. Try again in 4 days 2 hours 5 minutes.", now.Add(4*24*time.Hour + 2*time.Hour + 5*time.Minute)},
		{"limit reached, try again in 45 minutes", now.Add(45 * time.Minute)},
		{"Please try again in 1 hour and 30 minutes.", now.Add(90 * time.Minute)},
		{"resets in 2 hrs", now.Add(2 * time.Hour)},
		{"or try again at 3:41 PM.", at(2026, 10, 7, 15, 41)},
		{"or try again at 9:05 AM.", at(2026, 10, 8, 9, 5)}, // 9:05 has already passed today
		{"5-hour limit reached ∙ resets 7pm", at(2026, 10, 7, 19, 0)},
		{"limit reached. Resets at 12 am", at(2026, 10, 8, 0, 0)},
		{"try again at 12:00 PM", at(2026, 10, 8, 12, 0)},
		{"try again at Oct 9th, 2026 3:41 PM.", at(2026, 10, 9, 15, 41)},
		{"try again on Nov 2 at 10:00 AM", at(2026, 11, 2, 10, 0)},
		{"try again at Jan 5th 8:00 AM", at(2027, 1, 5, 8, 0)}, // no year and already past this year
		{fmt.Sprintf("Claude AI usage limit reached|%d", now.Add(2*time.Hour).Unix()), now.Add(2 * time.Hour)},
	}
	for _, c := range cases {
		got := ParseReset(c.msg, now)
		if !got.Equal(c.want) {
			t.Errorf("ParseReset(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
	for _, msg := range []string{
		"usage limit reached", "something else entirely", "try again at 25:99 PM", "Claude usage limit reached|1000000000",
		"You've hit your session limit · resets 3pm (Not/AZone)", // a zone this machine cannot load: say "unknown", do not guess
	} {
		if got := ParseReset(msg, now); !got.IsZero() {
			t.Errorf("ParseReset(%q) = %v, want zero (unknown)", msg, got)
		}
	}
}

// Claude Code writes "resets 3pm (America/New_York)": a clock time in the zone it names, which is not
// necessarily the zone Legatus runs in. Reading it in Legatus's zone would be hours wrong.
func TestParseResetReadsTheZoneClaudeCodeNames(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	// 2:00 pm in New York on 7 October 2026, written in UTC.
	now := time.Date(2026, 10, 7, 14, 0, 0, 0, ny).UTC()
	cases := []struct {
		msg  string
		want time.Time
	}{
		{"You've hit your session limit · resets 3pm (America/New_York)", time.Date(2026, 10, 7, 15, 0, 0, 0, ny)},
		{"You've hit your session limit · resets 1pm (America/New_York)", time.Date(2026, 10, 8, 13, 0, 0, 0, ny)},
		{"You've hit your Opus limit · resets 9:30am (America/New_York)", time.Date(2026, 10, 8, 9, 30, 0, 0, ny)},
		{"You've hit your weekly limit · resets Oct 12, 3pm (America/New_York)", time.Date(2026, 10, 12, 15, 0, 0, 0, ny)},
		{"You've hit your weekly limit · resets Oct 12 at 3pm (America/New_York)", time.Date(2026, 10, 12, 15, 0, 0, 0, ny)},
		{"You've hit your weekly limit · resets Jan 5, 2027, 8am (Asia/Tokyo)", time.Date(2027, 1, 5, 8, 0, 0, 0, tokyo)},
		{"You've hit your session limit · resets 3pm (UTC)", time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC)}, // 18:00 UTC has passed 3pm
	}
	for _, c := range cases {
		if got := ParseReset(c.msg, now); !got.Equal(c.want) {
			t.Errorf("ParseReset(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}
