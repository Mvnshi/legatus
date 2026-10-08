package automation

import (
	"strings"
	"testing"
	"time"
)

// October 2026: the 5th is a Monday, the 9th a Friday, the 10th a Saturday, the 11th a Sunday.
func at(day, hour, min int) time.Time { return time.Date(2026, 10, day, hour, min, 0, 0, time.UTC) }

func mustParse(t *testing.T, s string) Schedule {
	t.Helper()
	sched, err := ParseSchedule(s)
	if err != nil {
		t.Fatalf("ParseSchedule(%q): %v", s, err)
	}
	return sched
}

func TestParseSchedule(t *testing.T) {
	for _, ok := range []string{"every 5m", "every 30m", "every 6h", "every 2d", "daily 09:00", "daily 9:05", "Weekdays 17:30", "weekly mon 09:00", "weekly SUN 23:59", "  daily   09:00  "} {
		if _, err := ParseSchedule(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "every 1m", "every 4m", "every 0h", "every h", "daily 24:00", "daily 09:60", "weekly funday 09:00", "weekly 09:00", "monthly 09:00", "at noon", "every 6 weeks"} {
		if _, err := ParseSchedule(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	if _, err := ParseSchedule("every 1m"); err == nil || !strings.Contains(err.Error(), "at most every 5m") {
		t.Errorf("a too-frequent schedule should say why: %v", err)
	}
}

func TestIntervalSchedulesAreDueAfterTheInterval(t *testing.T) {
	s := mustParse(t, "every 6h")
	if s.Due(at(7, 10, 0), at(7, 15, 59)) || !s.Due(at(7, 10, 0), at(7, 16, 0)) || !s.Due(at(7, 10, 0), at(9, 10, 0)) {
		t.Fatal("an interval schedule is due once the interval has passed, and only then")
	}
}

func TestDailySchedulesRunOncePerDayAndCatchUpOnlyOnce(t *testing.T) {
	s := mustParse(t, "daily 09:00")
	cases := []struct {
		name      string
		last, now time.Time
		want      bool
	}{
		{"before today's time", at(6, 10, 0), at(7, 8, 59), false},
		{"at today's time", at(6, 10, 0), at(7, 9, 0), true},
		{"later the same day, already ran at nine", at(7, 9, 0), at(7, 12, 0), false},
		{"ran late last night", at(6, 23, 0), at(7, 8, 0), false},
		{"missed several days: owed, once", at(3, 10, 0), at(7, 10, 0), true},
		{"first seen at three, same afternoon", at(7, 15, 0), at(7, 16, 0), false},
		{"first seen at three, next morning", at(7, 15, 0), at(8, 9, 1), true},
	}
	for _, c := range cases {
		if got := s.Due(c.last, c.now); got != c.want {
			t.Errorf("%s: Due(%v, %v) = %v, want %v", c.name, c.last, c.now, got, c.want)
		}
	}
}

func TestWeekdaysSkipTheWeekendAndWeeklyUsesOneDay(t *testing.T) {
	wd := mustParse(t, "weekdays 09:00")
	if wd.Due(at(9, 9, 30), at(10, 12, 0)) || wd.Due(at(9, 9, 30), at(11, 12, 0)) {
		t.Error("weekdays ran on a weekend")
	}
	if !wd.Due(at(9, 9, 30), at(12, 9, 0)) {
		t.Error("weekdays did not run on Monday")
	}
	wk := mustParse(t, "weekly mon 09:00")
	if wk.Due(at(5, 9, 30), at(7, 12, 0)) {
		t.Error("weekly ran on a Wednesday")
	}
	if !wk.Due(at(5, 9, 30), at(12, 9, 0)) {
		t.Error("weekly did not run the next Monday")
	}
}

func TestNext(t *testing.T) {
	cases := []struct {
		schedule string
		after    time.Time
		want     time.Time
	}{
		{"every 6h", at(7, 10, 0), at(7, 16, 0)},
		{"daily 09:00", at(7, 10, 0), at(8, 9, 0)},
		{"daily 09:00", at(7, 8, 0), at(7, 9, 0)},
		{"weekdays 09:00", at(9, 10, 0), at(12, 9, 0)},
		{"weekly wed 09:00", at(7, 10, 0), at(14, 9, 0)},
		{"weekly wed 09:00", at(7, 8, 0), at(7, 9, 0)},
	}
	for _, c := range cases {
		if got := mustParse(t, c.schedule).Next(c.after); !got.Equal(c.want) {
			t.Errorf("%s after %v: Next = %v, want %v", c.schedule, c.after, got, c.want)
		}
	}
}
