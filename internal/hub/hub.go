// Package hub tells interested parties that something changed, without carrying the change itself. A
// listener waits on a channel, wakes when it is closed, and then reads what it needs from the store. That
// keeps live views free of lost or duplicated events: the journal on disk is the only source of truth.
package hub

import "sync"

// Hub is safe for concurrent use.
type Hub struct {
	mu   sync.Mutex
	any  chan struct{}
	runs map[string]chan struct{}
}

// New returns a Hub.
func New() *Hub {
	return &Hub{any: make(chan struct{}), runs: map[string]chan struct{}{}}
}

// Changed returns a channel that is closed the next time anything changes.
func (h *Hub) Changed() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.any
}

// RunChanged returns a channel that is closed the next time the given run changes. Take the channel before
// reading the run's state, then wait on it: a change that lands in between is not missed.
func (h *Hub) RunChanged(id string) <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.runs[id]
	if !ok {
		ch = make(chan struct{})
		h.runs[id] = ch
	}
	return ch
}

// Notify wakes everyone waiting on the run and everyone waiting on any change.
func (h *Hub) Notify(runID string) {
	h.mu.Lock()
	close(h.any)
	h.any = make(chan struct{})
	if ch, ok := h.runs[runID]; ok {
		close(ch)
		delete(h.runs, runID) // a fresh channel is made on the next RunChanged
	}
	h.mu.Unlock()
}
