package pool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newPool(t *testing.T, accounts ...Account) (*Pool, *clock) {
	t.Helper()
	c := &clock{t: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	p := New()
	p.Now = c.now
	for _, a := range accounts {
		if err := p.Add(a); err != nil {
			t.Fatal(err)
		}
	}
	return p, c
}

func acquire(t *testing.T, p *Pool, f Filter) *Lease {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	l, err := p.Acquire(ctx, f)
	if err != nil {
		t.Fatalf("Acquire(%+v): %v", f, err)
	}
	return l
}

func TestWorkSpreadsAcrossAccounts(t *testing.T) {
	p, _ := newPool(t, Account{ID: "a", Provider: "codex"}, Account{ID: "b", Provider: "codex"}, Account{ID: "c", Provider: "codex"})
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		seen[acquire(t, p, Filter{}).Account.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("three simultaneous jobs used %v, want all three accounts", seen)
	}
}

func TestAnAccountAtItsConcurrencyLimitMakesWorkWaitUntilItIsReleased(t *testing.T) {
	p, _ := newPool(t, Account{ID: "a", Provider: "codex"})
	first := acquire(t, p, Filter{})
	got := make(chan string, 1)
	go func() {
		l, err := p.Acquire(context.Background(), Filter{})
		if err != nil {
			got <- "error: " + err.Error()
			return
		}
		got <- l.Account.ID
	}()
	select {
	case id := <-got:
		t.Fatalf("second acquire did not wait: %s", id)
	case <-time.After(100 * time.Millisecond):
	}
	first.Release()
	select {
	case id := <-got:
		if id != "a" {
			t.Fatalf("got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiting acquire never woke after a release")
	}
}

func TestALimitedAccountIsSkippedAndTheNextOneIsUsed(t *testing.T) {
	p, c := newPool(t, Account{ID: "a", Provider: "codex"}, Account{ID: "b", Provider: "codex"})
	l := acquire(t, p, Filter{Prefer: "a"})
	if l.Account.ID != "a" {
		t.Fatalf("Prefer ignored: %s", l.Account.ID)
	}
	until := l.ReportLimit(c.now().Add(2 * time.Hour))
	if want := c.now().Add(2 * time.Hour); !until.Equal(want) {
		t.Fatalf("limited until %v, want %v", until, want)
	}
	for i := 0; i < 3; i++ {
		x := acquire(t, p, Filter{Prefer: "a"})
		if x.Account.ID != "b" {
			t.Fatalf("a limited account was used: %s", x.Account.ID)
		}
		x.Release()
	}
}

func TestWhenEveryAccountIsLimitedTheCallerLearnsWhenCapacityReturns(t *testing.T) {
	p, c := newPool(t, Account{ID: "a", Provider: "codex"}, Account{ID: "b", Provider: "codex"})
	a := acquire(t, p, Filter{Prefer: "a"})
	b := acquire(t, p, Filter{Prefer: "b"})
	a.ReportLimit(c.now().Add(3 * time.Hour))
	b.ReportLimit(c.now().Add(1 * time.Hour))
	_, err := p.Acquire(context.Background(), Filter{})
	var ce *CapacityError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a CapacityError", err)
	}
	if want := c.now().Add(1 * time.Hour); !ce.Until.Equal(want) {
		t.Fatalf("Until = %v, want the earlier reset %v", ce.Until, want)
	}
	c.advance(61 * time.Minute)
	l := acquire(t, p, Filter{})
	if l.Account.ID != "b" {
		t.Fatalf("after b reset, got %s", l.Account.ID)
	}
}

func TestALimitErrorWithoutAResetTimeUsesTheDefaultBackoff(t *testing.T) {
	p, c := newPool(t, Account{ID: "a", Provider: "codex"})
	l := acquire(t, p, Filter{})
	until := l.ReportLimit(time.Time{})
	if want := c.now().Add(DefaultLimitBackoff); !until.Equal(want) {
		t.Fatalf("until = %v, want %v", until, want)
	}
	// A reset time already in the past is the same as not knowing.
	l2 := (func() *Lease { c.advance(DefaultLimitBackoff + time.Second); return acquire(t, p, Filter{}) })()
	until = l2.ReportLimit(c.now().Add(-time.Minute))
	if want := c.now().Add(DefaultLimitBackoff); !until.Equal(want) {
		t.Fatalf("past reset: until = %v, want %v", until, want)
	}
}

func TestFiltersByProviderExclusionsAndDisabled(t *testing.T) {
	p, _ := newPool(t,
		Account{ID: "x", Provider: "codex"},
		Account{ID: "y", Provider: "claude"},
		Account{ID: "z", Provider: "claude", Disabled: true},
	)
	pickID := func(f Filter) string {
		l := acquire(t, p, f)
		defer l.Release()
		return l.Account.ID
	}
	if got := pickID(Filter{Providers: []string{"claude"}}); got != "y" {
		t.Fatalf("provider filter: %s", got)
	}
	if got := pickID(Filter{ExcludeProvider: "claude"}); got != "x" {
		t.Fatalf("exclude provider: %s", got)
	}
	if got := pickID(Filter{ExcludeAccounts: map[string]bool{"x": true}}); got != "y" {
		t.Fatalf("exclude account: %s", got)
	}
	_, err := p.Acquire(context.Background(), Filter{Providers: []string{"gemini"}})
	if !errors.Is(err, ErrNoAccounts) {
		t.Fatalf("unknown provider: %v", err)
	}
	_, err = p.Acquire(context.Background(), Filter{Providers: []string{"claude"}, ExcludeAccounts: map[string]bool{"y": true}})
	if !errors.Is(err, ErrNoAccounts) {
		t.Fatalf("only a disabled account left: %v", err)
	}
}

func TestMoreRemainingUsageIsPreferred(t *testing.T) {
	p, c := newPool(t, Account{ID: "a", Provider: "codex"}, Account{ID: "b", Provider: "codex"})
	p.SetUsage("a", 20)
	p.SetUsage("b", 90)
	c.advance(time.Minute)
	if got := acquire(t, p, Filter{}).Account.ID; got != "b" {
		t.Fatalf("picked %s with less usage left", got)
	}
}

func TestCancellationWhileWaitingReturnsTheContextError(t *testing.T) {
	p, _ := newPool(t, Account{ID: "a", Provider: "codex"})
	acquire(t, p, Filter{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Acquire(ctx, Filter{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestLimitsSurviveARestartAndExpiredOnesDoNot(t *testing.T) {
	p, c := newPool(t, Account{ID: "a", Provider: "codex"}, Account{ID: "b", Provider: "codex"})
	p.Add(Account{ID: "a", Provider: "codex"})
	la := acquire(t, p, Filter{Prefer: "a"})
	lb := acquire(t, p, Filter{Prefer: "b"})
	la.ReportLimit(c.now().Add(time.Hour))
	lb.ReportLimit(c.now().Add(time.Minute))
	saved := p.Snapshots()

	c.advance(10 * time.Minute)
	q := New()
	q.Now = c.now
	q.Restore(saved)
	l := (func() *Lease {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		l, err := q.Acquire(ctx, Filter{})
		if err != nil {
			t.Fatal(err)
		}
		return l
	})()
	if l.Account.ID != "b" {
		t.Fatalf("after restart got %s; a is still limited, b's limit expired", l.Account.ID)
	}
	l.Release()
	for _, s := range q.Snapshots() {
		if s.ID == "a" && s.LimitedUntil.IsZero() {
			t.Fatal("a's limit was lost across the restart")
		}
		if s.ID == "b" && !s.LimitedUntil.IsZero() {
			t.Fatal("b's expired limit was kept")
		}
	}
}

func TestReleaseAndReportAreIdempotent(t *testing.T) {
	p, _ := newPool(t, Account{ID: "a", Provider: "codex", MaxConcurrent: 2})
	l := acquire(t, p, Filter{})
	l.Release()
	l.Release()
	l.ReportLimit(time.Time{})
	if got := p.Snapshots()[0]; got.Active != 0 || !got.LimitedUntil.IsZero() {
		t.Fatalf("state after repeated calls: %+v", got)
	}
}

func TestManyWorkersNeverExceedAnAccountsConcurrency(t *testing.T) {
	p := New()
	p.Add(Account{ID: "a", Provider: "codex", MaxConcurrent: 2})
	p.Add(Account{ID: "b", Provider: "codex", MaxConcurrent: 1})
	var mu sync.Mutex
	active := map[string]int{}
	peak := map[string]int{}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := p.Acquire(context.Background(), Filter{})
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			active[l.Account.ID]++
			if active[l.Account.ID] > peak[l.Account.ID] {
				peak[l.Account.ID] = active[l.Account.ID]
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			active[l.Account.ID]--
			mu.Unlock()
			l.Release()
		}()
	}
	wg.Wait()
	if peak["a"] > 2 || peak["b"] > 1 {
		t.Fatalf("concurrency limits exceeded: %v", peak)
	}
	if peak["a"] == 0 || peak["b"] == 0 {
		t.Fatalf("an account was never used: %v", peak)
	}
}
