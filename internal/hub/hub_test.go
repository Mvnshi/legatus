package hub

import (
	"sync"
	"testing"
	"time"
)

func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-time.After(500 * time.Millisecond):
		return false
	}
}

func TestNotifyWakesTheRunAndTheGlobalListener(t *testing.T) {
	h := New()
	run, other, any := h.RunChanged("aaaa1111"), h.RunChanged("bbbb2222"), h.Changed()
	h.Notify("aaaa1111")
	if !closed(run) || !closed(any) {
		t.Fatal("the run's listener and the global listener should wake")
	}
	select {
	case <-other:
		t.Fatal("a listener for another run woke")
	default:
	}
}

func TestEachWaitSeesOnlyTheNextChange(t *testing.T) {
	h := New()
	first := h.RunChanged("aaaa1111")
	h.Notify("aaaa1111")
	second := h.RunChanged("aaaa1111")
	select {
	case <-second:
		t.Fatal("a new wait returned for a change that had already happened")
	default:
	}
	h.Notify("aaaa1111")
	if !closed(first) || !closed(second) {
		t.Fatal("both waits should have ended")
	}
}

func TestManyListenersAndNotifiersAtOnce(t *testing.T) {
	h := New()
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
				h.Notify("aaaa1111")
			}
		}
	}()
	defer close(stop)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-h.RunChanged("aaaa1111") }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listeners were left waiting")
	}
}
