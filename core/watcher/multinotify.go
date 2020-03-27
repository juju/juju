// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"sync"
	"time"

	"gopkg.in/tomb.v2"
)

// MultiNotifyWatcher implements NotifyWatcher, combining
// multiple NotifyWatchers.
type MultiNotifyWatcher struct {
	tomb     tomb.Tomb
	watchers []NotifyWatcher
	changes  chan struct{}
}

// NewMultiNotifyWatcher creates a NotifyWatcher that combines
// each of the NotifyWatchers passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
// Subsequent events are not coalesced.
func NewMultiNotifyWatcher(w ...NotifyWatcher) *MultiNotifyWatcher {
	m := &MultiNotifyWatcher{
		watchers: w,
		changes:  make(chan struct{}),
	}
	var wg sync.WaitGroup
	wg.Add(len(w))
	staging := make(chan struct{})
	for _, w := range w {
		// Consume the first event of each watcher.
		<-w.Changes()
		go func(wCopy NotifyWatcher) {
			defer wg.Done()
			_ = wCopy.Wait()
		}(w)
		// Copy events from the watcher to the staging channel.
		go copyEvents(staging, w.Changes(), &m.tomb)
	}
	m.tomb.Go(func() error {
		m.loop(staging)
		wg.Wait()
		return nil
	})
	return m
}

// loop copies events from the input channel to the output channel,
// coalescing events by waiting a short time between receiving and
// sending.
func (w *MultiNotifyWatcher) loop(in <-chan struct{}) {
	defer close(w.changes)
	// out is initialised to m.changes to send the initial event.
	out := w.changes
	var timer <-chan time.Time
	for {
		select {
		case <-w.tomb.Dying():
			return
		case <-in:
			if timer == nil {
				// TODO(fwereade): 2016-03-17 lp:1558657
				timer = time.After(10 * time.Millisecond)
			}
		case <-timer:
			timer = nil
			out = w.changes
		case out <- struct{}{}:
			out = nil
		}
	}
}

// copyEvents copies channel events from "in" to "out", coalescing.
func copyEvents(out chan<- struct{}, in <-chan struct{}, tomb *tomb.Tomb) {
	var outC chan<- struct{}
	for {
		select {
		case <-tomb.Dying():
			return
		case _, ok := <-in:
			if !ok {
				return
			}
			outC = out
		case outC <- struct{}{}:
			outC = nil
		}
	}
}

func (w *MultiNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
	for _, w := range w.watchers {
		w.Kill()
	}
}

func (w *MultiNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *MultiNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *MultiNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *MultiNotifyWatcher) Changes() NotifyChannel {
	return w.changes
}
