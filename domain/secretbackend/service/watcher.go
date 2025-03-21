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

type secretBackendRotateWatcher struct {
	catacomb      catacomb.Catacomb
	sourceWatcher watcher.StringsWatcher
	logger        logger.Logger

	processChanges func(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error)

	out chan []watcher.SecretBackendRotateChange
}

func newSecretBackendRotateWatcher(
	sourceWatcher watcher.StringsWatcher, logger logger.Logger,
	processChanges func(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error),
) (*secretBackendRotateWatcher, error) {
	w := &secretBackendRotateWatcher{
		sourceWatcher:  sourceWatcher,
		logger:         logger,
		processChanges: processChanges,
		out:            make(chan []watcher.SecretBackendRotateChange),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Capture(err)
}

func (w *secretBackendRotateWatcher) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *secretBackendRotateWatcher) loop() (err error) {
	defer close(w.out)

	ctx, cancel := w.scopedContext()
	defer cancel()

	// To allow the initial event to be sent.
	out := w.out
	var changes []watcher.SecretBackendRotateChange
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
func (w *secretBackendRotateWatcher) Changes() <-chan []watcher.SecretBackendRotateChange {
	return w.out
}

// Kill (worker.Worker) kills the watcher via its catacomb.
func (w *secretBackendRotateWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's catacomb to die,
// and returns the error with which it was killed.
func (w *secretBackendRotateWatcher) Wait() error {
	return w.catacomb.Wait()
}
