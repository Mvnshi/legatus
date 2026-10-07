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
	for _, msg := range []string{"usage limit reached", "something else entirely", "try again at 25:99 PM", "Claude usage limit reached|1000000000"} {
		if got := ParseReset(msg, now); !got.IsZero() {
			t.Errorf("ParseReset(%q) = %v, want zero (unknown)", msg, got)
		}
	}
}
