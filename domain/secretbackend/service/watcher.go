// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/watcher"
)

type secretBackendRotateWatcher struct {
	tomb          tomb.Tomb
	sourceWatcher watcher.StringsWatcher
	logger        Logger

	processChanges func(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error)

	out chan []watcher.SecretBackendRotateChange
}

func newSecretBackendRotateWatcher(
	sourceWatcher watcher.StringsWatcher, logger Logger,
	processChanges func(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error),
) *secretBackendRotateWatcher {
	w := &secretBackendRotateWatcher{
		sourceWatcher:  sourceWatcher,
		logger:         logger,
		processChanges: processChanges,
		out:            make(chan []watcher.SecretBackendRotateChange),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

func (w *secretBackendRotateWatcher) loop() (err error) {
	// To allow the initial event to sent.
	out := w.out
	var changes []watcher.SecretBackendRotateChange
	ctx := w.tomb.Context(context.Background())
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case backendIDs, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			w.logger.Debugf("received secret backend rotation changes: %v", backendIDs)

			var err error
			changes, err = w.processChanges(ctx, backendIDs...)
			if err != nil {
				return errors.Trace(err)
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

// Kill (worker.Worker) kills the watcher via its tomb.
func (w *secretBackendRotateWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *secretBackendRotateWatcher) Wait() error {
	return w.tomb.Wait()
}
