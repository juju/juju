// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
)

// secretWatcher is a watcher that watches for secret changes to a set of strings.
type secretWatcher[T any] struct {
	catacomb catacomb.Catacomb
	logger   logger.Logger

	sourceWatcher  watcher.StringsWatcher
	processChanges func(ctx context.Context, events ...string) ([]T, error)

	out chan []T
}

// NewSecretStringWatcher returns a new secrets watcher from the source watcher
// and tracks already seen secret URIs to ensure they are only notified once
// during an event batch. This is possible when we track revision or consumer
// events, since a unique secret can generate several events in such cases.
func NewSecretStringWatcher[T any](
	sourceWatcher watcher.StringsWatcher, logger logger.Logger,
	processChanges func(ctx context.Context, events ...string) ([]T, error),
) (*secretWatcher[T], error) {
	w := &secretWatcher[T]{
		sourceWatcher:  sourceWatcher,
		logger:         logger,
		processChanges: processChanges,
		out:            make(chan []T),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "secret-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Capture(err)
}

func (w *secretWatcher[T]) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *secretWatcher[T]) loop() error {
	defer close(w.out)

	// Secret watchers must always emit an initial event before any deltas.
	// Send that empty initial state deterministically before reading from the
	// source watcher, otherwise a buffered source event can win the select and
	// be emitted first.
	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case w.out <- nil:
	}

	var (
		historyIDs set.Strings
		changes    []T
	)
	var out chan []T
	addChanges := func(events set.Strings) error {
		if len(events) == 0 {
			return nil
		}
		ctx, cancel := w.scopedContext()
		defer cancel()
		processed, err := w.processChanges(ctx, events.Values()...)
		if err != nil {
			return errors.Capture(err)
		}
		if len(processed) == 0 {
			return nil
		}

		if historyIDs == nil {
			historyIDs = set.NewStrings()
		}
		for _, v := range processed {
			id := fmt.Sprint(v)
			if historyIDs.Contains(id) {
				continue
			}
			changes = append(changes, v)
			historyIDs.Add(id)
		}

		if len(changes) != 0 {
			out = w.out
		}
		return nil
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case events, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			if err := addChanges(set.NewStrings(events...)); err != nil {
				return errors.Capture(err)
			}
		case out <- changes:
			historyIDs = nil
			changes = nil
			out = nil
		}
	}
}

// Changes returns the channel of secret changes.
func (w *secretWatcher[T]) Changes() <-chan []T {
	return w.out
}

// Stop stops the watcher.
func (w *secretWatcher[T]) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher via its tomb.
func (w *secretWatcher[T]) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *secretWatcher[T]) Wait() error {
	return w.catacomb.Wait()
}
