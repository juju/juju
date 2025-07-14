// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
)

// stringSourcedWatcher is a generic watcher that listens to changes from a source
// watcher and maps the events to a specific type T using the provided mapper function.
// The mapper function should take the context and the events as input and return a slice of T or an error.
// The watcher will emit the mapped results on its Changes channel.
type stringSourcedWatcher[T any] struct {
	catacomb catacomb.Catacomb

	sourceWatcher watcher.StringsWatcher
	mapper        func(ctx context.Context, events ...string) ([]T, error)

	out chan []T
}

func newStringSourcedWatcher[T any](
	sourceWatcher watcher.StringsWatcher,
	mapper func(ctx context.Context, events ...string) ([]T, error),
) (*stringSourcedWatcher[T], error) {
	w := &stringSourcedWatcher[T]{
		sourceWatcher: sourceWatcher,
		mapper:        mapper,
		out:           make(chan []T, 1),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "attachment-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Capture(err)
}

func (w *stringSourcedWatcher[T]) loop() error {
	defer close(w.out)

	var (
		changes []T
		out     chan []T
	)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case events, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("source watcher %v closed", w.sourceWatcher)
			}

			if len(events) == 0 {
				continue
			}

			results, err := w.mapper(w.catacomb.Context(context.Background()), events...)
			if err != nil {
				return errors.Errorf("processing changes: %w", err)
			}

			if len(results) == 0 {
				continue
			}

			changes = append(changes, results...)
			// If we have changes, we need to dispatch them.
			out = w.out
		case out <- changes:
			// We have dispatched. Reset changes for the next batch.
			changes = nil
			out = nil
		}
	}
}

// Changes returns the channel of the changes.
func (w *stringSourcedWatcher[T]) Changes() <-chan []T {
	return w.out
}

// Stop stops the watcher.
func (w *stringSourcedWatcher[T]) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher via its tomb.
func (w *stringSourcedWatcher[T]) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *stringSourcedWatcher[T]) Wait() error {
	return w.catacomb.Wait()
}
