// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/internal/errors"
)

// StringsNotifyWatcher wraps a Watcher[[]string] and provides a
// Watcher[struct{}] interface.
type StringsNotifyWatcher struct {
	catacomb catacomb.Catacomb
	out      chan struct{}
	watcher  Watcher[[]string]
}

// NewStringsNotifyWatcher creates a new StringsNotifyWatcher.
func NewStringsNotifyWatcher(watcher Watcher[[]string]) (*StringsNotifyWatcher, error) {
	w := StringsNotifyWatcher{
		watcher: watcher,
		out:     make(chan struct{}),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "strings-notify-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{watcher},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return &w, nil
}

func (w *StringsNotifyWatcher) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.watcher.Changes():
			if !ok {
				return nil
			}

			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.out <- struct{}{}:
			}
		}
	}
}

func (w *StringsNotifyWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *StringsNotifyWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *StringsNotifyWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *StringsNotifyWatcher) Err() error {
	return w.catacomb.Err()
}

func (w *StringsNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

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
// Deprecated: do not use, use NewNotifyMapperWatcher instead.
func NewMultiNotifyWatcher(ctx context.Context, watchers ...Watcher[struct{}]) (*MultiWatcher[struct{}], error) {
	applier := func(_, _ struct{}) struct{} {
		return struct{}{}
	}
	return newMultiWatcher[struct{}](ctx, applier, watchers...)
}

// newMultiWatcher creates a NotifyWatcher that combines
// each of the NotifyWatchers passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
// Subsequent events are not coalesced.
// Deprecated: delete when NewMultiNotifyWatcher is gone.
func newMultiWatcher[T any](ctx context.Context, applier Applier[T], watchers ...Watcher[T]) (*MultiWatcher[T], error) {
	workers := make([]worker.Worker, len(watchers))
	for i, w := range watchers {
		_, err := ConsumeInitialEvent[T](ctx, w)
		if err != nil {
			return nil, errors.Capture(err)
		}

		workers[i] = w
	}

	w := &MultiWatcher[T]{
		staging: make(chan T),
		changes: make(chan T),
		applier: applier,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "multi-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: workers,
	}); err != nil {
		return nil, errors.Capture(err)
	}

	for _, watcher := range watchers {
		// Copy events from the watcher to the staging channel.
		go w.copyEvents(watcher.Changes())
	}

	return w, nil
}

// loop copies events from the input channel to the output channel.
func (w *MultiWatcher[T]) loop() error {
	defer close(w.changes)

	var in <-chan T
	var payload T
	out := w.changes
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case v := <-in:
			payload = w.applier(payload, v)

			out = w.changes
			in = nil
		case out <- payload:
			out = nil
			in = w.staging

			// Ensure we reset the payload to the initial value after
			// sending it to the channel.
			var init T
			payload = init
		}
	}
}

// copyEvents copies channel events from "in" to "out".
func (w *MultiWatcher[T]) copyEvents(in <-chan T) {
	var (
		outC    chan<- T
		payload T
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return
		case v, ok := <-in:
			if !ok {
				return
			}
			payload = w.applier(payload, v)
			outC = w.staging
		case outC <- payload:
			outC = nil

			// Ensure we reset the payload to the initial value after
			// sending it to the channel.
			var init T
			payload = init
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
