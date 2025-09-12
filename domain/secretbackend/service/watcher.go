// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
)

type secretBackendRotateWatcher[T any] struct {
	catacomb      catacomb.Catacomb
	sourceWatcher watcher.StringsWatcher
	logger        logger.Logger

	processChanges func(ctx context.Context, backendIDs ...string) ([]T, error)

	out chan []T
}

func newSecretBackendRotateWatcher[T any](
	sourceWatcher watcher.StringsWatcher, logger logger.Logger,
	processChanges func(ctx context.Context, backendIDs ...string) ([]T, error),
) (*secretBackendRotateWatcher[T], error) {
	w := &secretBackendRotateWatcher[T]{
		sourceWatcher:  sourceWatcher,
		logger:         logger,
		processChanges: processChanges,
		out:            make(chan []T),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "secret-backend-rotate-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Capture(err)
}

func (w *secretBackendRotateWatcher[T]) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *secretBackendRotateWatcher[T]) loop() (err error) {
	defer close(w.out)

	ctx, cancel := w.scopedContext()
	defer cancel()

	// To allow the initial event to be sent.
	out := w.out
	var changes []T
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case backendIDs, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			w.logger.Debugf(ctx, "received secret backend rotation changes: %v", backendIDs)

			var err error
			changes, err = w.processChanges(ctx, backendIDs...)
			if err != nil {
				return errors.Capture(err)
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			out = nil
		}
	}
}

// Changes returns the channel of secret backend rotation changes.
func (w *secretBackendRotateWatcher[T]) Changes() <-chan []T {
	return w.out
}

// Kill (worker.Worker) kills the watcher via its catacomb.
func (w *secretBackendRotateWatcher[T]) Kill() {
	w.catacomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's catacomb to die,
// and returns the error with which it was killed.
func (w *secretBackendRotateWatcher[T]) Wait() error {
	return w.catacomb.Wait()
}
