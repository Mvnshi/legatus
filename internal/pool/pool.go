// Package pool is the account scheduler: it hands each piece of agent work to one of the user's own
// logins that still has usage, sets an account aside when it hits its limit until it resets, and says
// when capacity will be back if every account is out.
//
// An Account is a login of a coding agent kept in its own configuration directory (for example a Codex
// home or a Claude config directory), so many logins can be used side by side without switching.
package pool

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Account is one login of one agent.
type Account struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"` // "codex", "claude", ...
	Label         string `json:"label,omitempty"`
	Home          string `json:"home,omitempty"` // the agent's configuration directory for this login
	MaxConcurrent int    `json:"max_concurrent,omitempty"`
	Disabled      bool   `json:"disabled,omitempty"`
}

func (a Account) max() int {
	if a.MaxConcurrent <= 0 {
		return 1
	}
	return a.MaxConcurrent
}

// Snapshot is an account plus its live state; it is what gets saved and shown.
type Snapshot struct {
	Account
	Active       int       `json:"active"`
	LimitedUntil time.Time `json:"limited_until,omitempty"`
	RemainingPct *float64  `json:"remaining_pct,omitempty"` // known only when a usage probe reported it
	LastUsed     time.Time `json:"last_used,omitempty"`
	Uses         int       `json:"uses"`
}

// Filter says which accounts a piece of work may use.
type Filter struct {
	Providers       []string        // empty means any provider
	ExcludeProvider string          // never use this provider
	ExcludeAccounts map[string]bool // never use these accounts
	Prefer          string          // an account to use if it is available (keeps a chat on one login)
}

// ErrNoAccounts means no account could ever serve the filter (none configured, or all disabled/excluded).
var ErrNoAccounts = errors.New("no account matches")

// CapacityError means every matching account is at its usage limit.
type CapacityError struct {
	Until time.Time // the earliest moment one of them resets
}

func (e *CapacityError) Error() string {
	return fmt.Sprintf("every matching account is at its usage limit until %s", e.Until.Format(time.RFC3339))
}

// DefaultLimitBackoff is how long an account is set aside when a limit error does not say when it resets.
const DefaultLimitBackoff = 30 * time.Minute

// Pool is safe for concurrent use.
type Pool struct {
	// Now is the clock; time.Now when nil. Tests replace it.
	Now func() time.Time
	// OnChange is called (without the pool's lock held) after the set of accounts or any limit changes, so
	// the caller can save a Snapshot.
	OnChange func()

	mu       sync.Mutex
	accounts map[string]*state
	order    []string
	changed  chan struct{}
}

type state struct {
	Snapshot
}

// New returns an empty pool.
func New() *Pool {
	return &Pool{accounts: map[string]*state{}, changed: make(chan struct{})}
}

func (p *Pool) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// notify wakes everyone waiting in Acquire. The caller holds p.mu.
func (p *Pool) notify() {
	close(p.changed)
	p.changed = make(chan struct{})
}

func (p *Pool) fire() {
	if p.OnChange != nil {
		p.OnChange()
	}
}

// Add registers an account, or updates an existing one's settings without losing its live state.
func (p *Pool) Add(a Account) error {
	if a.ID == "" || a.Provider == "" {
		return errors.New("an account needs an id and a provider")
	}
	p.mu.Lock()
	if s, ok := p.accounts[a.ID]; ok {
		s.Account = a
	} else {
		p.accounts[a.ID] = &state{Snapshot{Account: a}}
		p.order = append(p.order, a.ID)
	}
	p.notify()
	p.mu.Unlock()
	p.fire()
	return nil
}

// Remove forgets an account. Work already running on it is not interrupted.
func (p *Pool) Remove(id string) {
	p.mu.Lock()
	delete(p.accounts, id)
	for i, x := range p.order {
		if x == id {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
	p.notify()
	p.mu.Unlock()
	p.fire()
}

// SetUsage records how much usage an account has left, in percent, when a probe knows. The scheduler
// prefers accounts with more left.
func (p *Pool) SetUsage(id string, remainingPct float64) {
	p.mu.Lock()
	if s, ok := p.accounts[id]; ok {
		v := remainingPct
		s.RemainingPct = &v
	}
	p.mu.Unlock()
}

// Snapshots lists every account in the order it was added.
func (p *Pool) Snapshots() []Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Snapshot, 0, len(p.order))
	for _, id := range p.order {
		out = append(out, p.accounts[id].Snapshot)
	}
	return out
}

// Restore loads saved accounts and limits. Limits that have already passed are dropped; live counters
// of running work start at zero because nothing is running yet.
func (p *Pool) Restore(snaps []Snapshot) {
	p.mu.Lock()
	now := p.now()
	for _, sn := range snaps {
		s := &state{sn}
		s.Active = 0
		if !s.LimitedUntil.After(now) {
			s.LimitedUntil = time.Time{}
		}
		if _, ok := p.accounts[sn.ID]; !ok {
			p.order = append(p.order, sn.ID)
		}
		p.accounts[sn.ID] = s
	}
	p.notify()
	p.mu.Unlock()
}

// Lease is the right to run one piece of work on one account.
type Lease struct {
	Account Account
	pool    *Pool
	done    bool
	mu      sync.Mutex
}

func (l *Lease) finish() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.done {
		return false
	}
	l.done = true
	return true
}

// Release gives the account back after work that did not hit a limit.
func (l *Lease) Release() {
	if !l.finish() {
		return
	}
	p := l.pool
	p.mu.Lock()
	if s, ok := p.accounts[l.Account.ID]; ok && s.Active > 0 {
		s.Active--
	}
	p.notify()
	p.mu.Unlock()
}

// ReportLimit gives the account back and sets it aside until resetAt. A zero or past resetAt means "the
// error did not say", and the default backoff is used.
func (l *Lease) ReportLimit(resetAt time.Time) time.Time {
	if !l.finish() {
		return time.Time{}
	}
	p := l.pool
	p.mu.Lock()
	now := p.now()
	if !resetAt.After(now) {
		resetAt = now.Add(DefaultLimitBackoff)
	}
	if s, ok := p.accounts[l.Account.ID]; ok {
		if s.Active > 0 {
			s.Active--
		}
		if resetAt.After(s.LimitedUntil) {
			s.LimitedUntil = resetAt
		}
		resetAt = s.LimitedUntil
		zero := 0.0
		s.RemainingPct = &zero
	}
	p.notify()
	p.mu.Unlock()
	p.fire()
	return resetAt
}

type pickResult struct {
	lease         *state
	eligible      int
	busy          int
	earliestReset time.Time
}

func (f Filter) allows(s *state) bool {
	if s.Disabled || f.ExcludeAccounts[s.ID] {
		return false
	}
	if f.ExcludeProvider != "" && s.Provider == f.ExcludeProvider {
		return false
	}
	if len(f.Providers) == 0 {
		return true
	}
	for _, p := range f.Providers {
		if p == s.Provider {
			return true
		}
	}
	return false
}

// pick chooses an account. The caller holds p.mu.
func (p *Pool) pick(f Filter, now time.Time) pickResult {
	var res pickResult
	var usable []*state
	for _, id := range p.order {
		s := p.accounts[id]
		if !f.allows(s) {
			continue
		}
		res.eligible++
		if s.LimitedUntil.After(now) {
			if res.earliestReset.IsZero() || s.LimitedUntil.Before(res.earliestReset) {
				res.earliestReset = s.LimitedUntil
			}
			continue
		}
		if s.Active >= s.max() {
			res.busy++
			continue
		}
		usable = append(usable, s)
	}
	if len(usable) == 0 {
		return res
	}
	sort.SliceStable(usable, func(i, j int) bool {
		a, b := usable[i], usable[j]
		if f.Prefer != "" && (a.ID == f.Prefer) != (b.ID == f.Prefer) {
			return a.ID == f.Prefer
		}
		la, lb := float64(a.Active)/float64(a.max()), float64(b.Active)/float64(b.max())
		if la != lb {
			return la < lb
		}
		if a.RemainingPct != nil && b.RemainingPct != nil && *a.RemainingPct != *b.RemainingPct {
			return *a.RemainingPct > *b.RemainingPct
		}
		return a.LastUsed.Before(b.LastUsed)
	})
	res.lease = usable[0]
	return res
}

// Acquire waits until an account matching the filter can take work and leases it. It returns:
//   - ErrNoAccounts when nothing could ever match;
//   - *CapacityError when every matching account is at its limit (it does not wait, so the caller can
//     park the run until Until instead of holding a goroutine and a place in line);
//   - the context's error if it is cancelled while every matching account is merely busy.
func (p *Pool) Acquire(ctx context.Context, f Filter) (*Lease, error) {
	for {
		p.mu.Lock()
		now := p.now()
		res := p.pick(f, now)
		if res.lease != nil {
			s := res.lease
			s.Active++
			s.Uses++
			s.LastUsed = now
			lease := &Lease{Account: s.Account, pool: p}
			p.mu.Unlock()
			return lease, nil
		}
		if res.eligible == 0 {
			p.mu.Unlock()
			return nil, ErrNoAccounts
		}
		if res.busy == 0 {
			p.mu.Unlock()
			return nil, &CapacityError{Until: res.earliestReset}
		}
		wake := p.changed
		wait := time.Duration(-1)
		if !res.earliestReset.IsZero() {
			wait = res.earliestReset.Sub(now)
		}
		p.mu.Unlock()

		var timer <-chan time.Time
		var t *time.Timer
		if wait >= 0 {
			t = time.NewTimer(wait + 5*time.Millisecond)
			timer = t.C
		}
		select {
		case <-ctx.Done():
			if t != nil {
				t.Stop()
			}
			return nil, ctx.Err()
		case <-wake:
		case <-timer:
		}
		if t != nil {
			t.Stop()
		}
	}
}
