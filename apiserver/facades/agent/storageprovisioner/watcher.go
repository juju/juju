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

type attachmentWatcher[T any] struct {
	catacomb catacomb.Catacomb

	sourceWatcher  watcher.StringsWatcher
	processChanges func(ctx context.Context, events ...string) ([]T, error)

	out chan []T
}

func newAttachmentWatcher[T any](
	sourceWatcher watcher.StringsWatcher,
	processChanges func(ctx context.Context, events ...string) ([]T, error),
) (*attachmentWatcher[T], error) {
	w := &attachmentWatcher[T]{
		sourceWatcher:  sourceWatcher,
		processChanges: processChanges,
		out:            make(chan []T, 1),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "attachment-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Capture(err)
}

func (w *attachmentWatcher[T]) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *attachmentWatcher[T]) loop() error {
	defer close(w.out)

	ctx, cancel := w.scopedContext()
	defer cancel()

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

			results, err := w.processChanges(ctx, events...)
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
func (w *attachmentWatcher[T]) Changes() <-chan []T {
	return w.out
}

// Stop stops the watcher.
func (w *attachmentWatcher[T]) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher via its tomb.
func (w *attachmentWatcher[T]) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *attachmentWatcher[T]) Wait() error {
	return w.catacomb.Wait()
}
