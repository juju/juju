// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
)

// Applier is a function that applies a change to a value.
type Applier[T any] func(T, T) T

// MultiWatcher implements Watcher, combining multiple Watchers.
type MultiWatcher[T any] struct {
	catacomb         catacomb.Catacomb
	staging, changes chan T
	applier          Applier[T]
}

// NewMultiNotifyWatcher creates a NotifyWatcher that combines
// each of the NotifyWatchers passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
func NewMultiNotifyWatcher(ctx context.Context, watchers ...Watcher[struct{}]) (*MultiWatcher[struct{}], error) {
	applier := func(_, _ struct{}) struct{} {
		return struct{}{}
	}
	return NewMultiWatcher[struct{}](ctx, applier, watchers...)
}

// NewMultiStringsWatcher creates a StringsWatcher that combines
// each of the StringsWatcher passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
func NewMultiStringsWatcher(ctx context.Context, watchers ...Watcher[[]string]) (*MultiWatcher[[]string], error) {
	applier := func(current, additional []string) []string {
		return append(current, additional...)
	}
	return NewMultiWatcher[[]string](ctx, applier, watchers...)
}

// NewMultiNotifyWatcher creates a NotifyWatcher that combines
// each of the NotifyWatchers passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
// Subsequent events are not coalesced.
func NewMultiWatcher[T any](ctx context.Context, applier Applier[T], watchers ...Watcher[T]) (*MultiWatcher[T], error) {
	workers := make([]worker.Worker, len(watchers))
	for i, w := range watchers {
		_, err := ConsumeInitialEvent[T](ctx, w)
		if err != nil {
			return nil, errors.Trace(err)
		}

		workers[i] = w
	}

	w := &MultiWatcher[T]{
		staging: make(chan T),
		changes: make(chan T),
		applier: applier,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: workers,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	for _, watcher := range watchers {
		// Copy events from the watcher to the staging channel.
		go w.copyEvents(w.staging, watcher.Changes())
	}

	return w, nil
}

// loop copies events from the input channel to the output channel,
// coalescing events by waiting a short time between receiving and
// sending.
func (w *MultiWatcher[T]) loop() error {
	defer close(w.changes)

	out := w.changes
	var values T
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case values = <-w.staging:
			out = w.changes
		case out <- values:
			out = nil
		}
	}
}

// copyEvents copies channel events from "in" to "out", coalescing.
func (w *MultiWatcher[T]) copyEvents(out chan<- T, in <-chan T) {
	var (
		outC   chan<- T
		values T
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return
		case v, ok := <-in:
			if !ok {
				return
			}
			values = w.applier(values, v)
			outC = out
		case outC <- values:
			outC = nil
		}
	}
}

func (w *MultiWatcher[T]) Kill() {
	w.catacomb.Kill(nil)
}

func (w *MultiWatcher[T]) Wait() error {
	return w.catacomb.Wait()
}

func (w *MultiWatcher[T]) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *MultiWatcher[T]) Err() error {
	return w.catacomb.Err()
}

func (w *MultiWatcher[T]) Changes() <-chan T {
	return w.changes
}
