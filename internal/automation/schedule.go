// Package automation starts runs without being asked: on a schedule, and when a GitHub issue with a given
// label from a trusted author appears.
package automation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Schedule says when something is due. The grammar is deliberately small:
//
//	every 30m | every 6h | every 2d      at least every 5 minutes
//	daily 09:00                          every day at that local time
//	weekdays 09:00                       Monday to Friday
//	weekly mon 09:00                     one day a week
type Schedule struct {
	text    string
	every   time.Duration // for "every ..."
	hour    int
	minute  int
	weekday *time.Weekday // for "weekly ..."
	clock   bool          // a time-of-day schedule rather than an interval
	days    string        // "daily", "weekdays" or "weekly"
}

var (
	everyRE = regexp.MustCompile(`^every (\d+)\s*([mhd])$`)
	clockRE = regexp.MustCompile(`^(daily|weekdays|weekly (mon|tue|wed|thu|fri|sat|sun)) ([01]?\d|2[0-3]):([0-5]\d)$`)
	days    = map[string]time.Weekday{"mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday}
)

// MinInterval is the shortest repeat a schedule may ask for.
const MinInterval = 5 * time.Minute

// ParseSchedule reads a schedule.
func ParseSchedule(text string) (Schedule, error) {
	t := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if m := everyRE.FindStringSubmatch(t); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{"m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		d := time.Duration(n) * unit
		if d < MinInterval {
			return Schedule{}, fmt.Errorf("%q: a schedule may repeat at most every %s", text, MinInterval)
		}
		return Schedule{text: t, every: d}, nil
	}
	if m := clockRE.FindStringSubmatch(t); m != nil {
		h, _ := strconv.Atoi(m[3])
		min, _ := strconv.Atoi(m[4])
		s := Schedule{text: t, clock: true, hour: h, minute: min}
		switch {
		case m[1] == "daily":
			s.days = "daily"
		case m[1] == "weekdays":
			s.days = "weekdays"
		default:
			s.days = "weekly"
			d := days[m[2]]
			s.weekday = &d
		}
		return s, nil
	}
	return Schedule{}, fmt.Errorf("%q is not a schedule: use \"every 6h\", \"daily 09:00\", \"weekdays 09:00\" or \"weekly mon 09:00\"", text)
}

func (s Schedule) String() string { return s.text }

// allowed reports whether the schedule runs on that calendar day.
func (s Schedule) allowed(d time.Weekday) bool {
	switch s.days {
	case "weekdays":
		return d != time.Saturday && d != time.Sunday
	case "weekly":
		return d == *s.weekday
	}
	return true
}

// latest is the most recent scheduled moment at or before now (clock schedules only).
func (s Schedule) latest(now time.Time) time.Time {
	for back := 0; back < 8; back++ {
		day := now.AddDate(0, 0, -back)
		if !s.allowed(day.Weekday()) {
			continue
		}
		at := time.Date(day.Year(), day.Month(), day.Day(), s.hour, s.minute, 0, 0, now.Location())
		if !at.After(now) {
			return at
		}
	}
	return time.Time{}
}

// Due reports whether a run is owed: last is when it last ran (or when it was first seen). A schedule that
// was missed several times is owed one run, not several.
func (s Schedule) Due(last, now time.Time) bool {
	if s.clock {
		l := s.latest(now)
		return !l.IsZero() && l.After(last)
	}
	return !now.Before(last.Add(s.every))
}

// Next is when the schedule is next due after the given time.
func (s Schedule) Next(after time.Time) time.Time {
	if !s.clock {
		return after.Add(s.every)
	}
	for ahead := 0; ahead < 8; ahead++ {
		day := after.AddDate(0, 0, ahead)
		if !s.allowed(day.Weekday()) {
			continue
		}
		at := time.Date(day.Year(), day.Month(), day.Day(), s.hour, s.minute, 0, 0, after.Location())
		if at.After(after) {
			return at
		}
	}
	return time.Time{}
}
