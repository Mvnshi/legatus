package agent

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	// Claude Code names the time zone of a reset time ("resets 3pm (America/New_York)"). Windows has no zone
	// database of its own, so ship one with the program instead of misreading the hour.
	_ "time/tzdata"
)

// notALoginLimit are messages that look like limits but say outright that the login is fine. Claude Code
// prints "Server is temporarily limiting requests (not your usage limit)" when the service is throttling
// everyone; setting the login aside for that would idle a working account.
var notALoginLimit = []string{"not your usage limit", "not your rate limit", "temporarily limiting requests"}

// IsServerThrottle reports whether a message says the service is limiting everyone, not this login.
func IsServerThrottle(msg string) bool {
	s := strings.ToLower(msg)
	for _, p := range notALoginLimit {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// namedLimitRE matches "You've hit your session limit", "...your weekly limit", "...your Opus limit" and
// "...your org's monthly spend limit": the wording Claude Code 2.1.295 uses, where the name of the limit
// sits between "your" and "limit".
var namedLimitRE = regexp.MustCompile(`hit your [^.·\n]{1,40}\blimit\b`)

// IsLimitMessage reports whether an error message from a coding agent means the login is out of usage.
func IsLimitMessage(msg string) bool {
	if IsServerThrottle(msg) {
		return false
	}
	s := strings.ToLower(msg)
	if namedLimitRE.MatchString(s) {
		return true
	}
	for _, p := range []string{
		"usage limit", "usage_limit", "usagelimit", "limit reached", "hit your limit", "rate limit", "rate_limit",
		"quota", "out of credits", "insufficient credits", "credit balance is too low", "credit balance too low",
		"out of usage", "shared budget", "too many requests",
	} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

var (
	relativeRE = regexp.MustCompile(`(?i)(?:try again|resets?|available again|retry)\s+in\s+((?:\d+\s*(?:days?|d|hours?|hrs?|h|minutes?|mins?|m|seconds?|secs?|s)\b[\s,]*(?:and\s+)?)+)`)
	partRE     = regexp.MustCompile(`(?i)(\d+)\s*(days?|d|hours?|hrs?|h|minutes?|mins?|m|seconds?|secs?|s)\b`)
	// "try again at 3:41 PM", "resets 3pm", "resets at 7:30 pm", optionally with a date before the time.
	clockRE = regexp.MustCompile(`(?i)(?:try again|resets?|available again|retry)\s+(?:at\s+)?(?:(?:on\s+)?(?:(?P<mon>jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+(?P<day>\d{1,2})(?:st|nd|rd|th)?,?\s*(?:(?P<year>\d{4}),?\s*)?)(?:at\s+)?)?(?P<h>\d{1,2})(?::(?P<m>\d{2}))?\s*(?P<ap>am|pm)\b`)
	// "Claude AI usage limit reached|1760000000": a Unix time after a bar.
	epochRE = regexp.MustCompile(`\|\s*(\d{10})\b`)
	// "(America/New_York)" straight after the time: Claude Code names the zone it printed the time in.
	zoneRE = regexp.MustCompile(`^\s*\(([A-Za-z][A-Za-z0-9_+\-]*(?:/[A-Za-z0-9_+\-]+)*)\)`)
)

var months = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April, "may": time.May, "jun": time.June,
	"jul": time.July, "aug": time.August, "sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}

// ParseReset finds when a login's usage comes back in the text of a limit error. It understands the
// wordings the agents use ("try again in 4 days 2 hours", "try again at 3:41 PM", "resets 7pm", "resets
// Oct 12, 3pm (America/New_York)", a Unix time after a bar). Times without a date are read in the zone the
// message names, else in now's time zone, and mean the next such time. It returns the zero time when the
// message does not say, or names a zone it cannot load; the pool then uses its default backoff.
func ParseReset(msg string, now time.Time) time.Time {
	if m := epochRE.FindStringSubmatch(msg); m != nil {
		if sec, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			if t := time.Unix(sec, 0); t.After(now) {
				return t
			}
		}
	}
	if m := relativeRE.FindStringSubmatch(msg); m != nil {
		var d time.Duration
		for _, p := range partRE.FindAllStringSubmatch(m[1], -1) {
			n, _ := strconv.Atoi(p[1])
			switch strings.ToLower(p[2])[0] {
			case 'd':
				d += time.Duration(n) * 24 * time.Hour
			case 'h':
				d += time.Duration(n) * time.Hour
			case 'm':
				d += time.Duration(n) * time.Minute
			case 's':
				d += time.Duration(n) * time.Second
			}
		}
		if d > 0 {
			return now.Add(d)
		}
	}
	if idx := clockRE.FindStringSubmatchIndex(msg); idx != nil {
		m := make([]string, len(idx)/2)
		for i := range m {
			if idx[2*i] >= 0 {
				m[i] = msg[idx[2*i]:idx[2*i+1]]
			}
		}
		clockEnd := idx[1]
		g := func(name string) string { return m[clockRE.SubexpIndex(name)] }
		h, _ := strconv.Atoi(g("h"))
		min, _ := strconv.Atoi(g("m"))
		if h < 1 || h > 12 || min > 59 {
			return time.Time{}
		}
		if strings.EqualFold(g("ap"), "pm") && h != 12 {
			h += 12
		}
		if strings.EqualFold(g("ap"), "am") && h == 12 {
			h = 0
		}
		loc := now.Location()
		if z := zoneRE.FindStringSubmatch(msg[clockEnd:]); z != nil {
			l, err := time.LoadLocation(z[1])
			if err != nil {
				return time.Time{}
			}
			loc = l
			now = now.In(loc)
		}
		if mon := g("mon"); mon != "" {
			day, _ := strconv.Atoi(g("day"))
			year := now.Year()
			if y := g("year"); y != "" {
				year, _ = strconv.Atoi(y)
			}
			t := time.Date(year, months[strings.ToLower(mon[:3])], day, h, min, 0, 0, loc)
			if g("year") == "" && !t.After(now) {
				t = t.AddDate(1, 0, 0)
			}
			return t
		}
		t := time.Date(now.Year(), now.Month(), now.Day(), h, min, 0, 0, loc)
		if !t.After(now) {
			t = t.AddDate(0, 0, 1)
		}
		return t
	}
	return time.Time{}
}
