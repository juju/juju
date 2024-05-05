// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/watcher"
)

type secretBackendModelConfigWatcher struct {
	catacomb      catacomb.Catacomb
	out           chan struct{}
	src           watcher.NotifyWatcher
	backendGetter SecretBackendGetter
	logger        loggo.Logger

	currentSecretBackend string
}

func newSecretBackendModelConfigWatcher(ctx context.Context, backendGetter SecretBackendGetter, src watcher.NotifyWatcher, logger loggo.Logger) (watcher.NotifyWatcher, error) {
	w := &secretBackendModelConfigWatcher{
		out:           make(chan struct{}),
		src:           src,
		backendGetter: backendGetter,
		logger:        logger,
	}
	var err error
	w.currentSecretBackend, err = w.backendGetter.GetSecretBackendID(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{src},
	})
	return w, errors.Trace(err)
}

func (w *secretBackendModelConfigWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *secretBackendModelConfigWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *secretBackendModelConfigWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *secretBackendModelConfigWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *secretBackendModelConfigWatcher) Err() error {
	return w.catacomb.Err()
}

func (w *secretBackendModelConfigWatcher) loop() error {
	defer close(w.out)

	var out chan struct{}

	// We want to send the initial event anyway.
	sendInitial := true

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.src.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			changed, err := w.processModelChange()
			if err != nil {
				return errors.Trace(err)
			}
			if changed || sendInitial {
				out = w.out
			}
		case out <- struct{}{}:
			out = nil
			sendInitial = false
		}
	}
}

// scopedContext returns a context that is in the scope of the watcher lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *secretBackendModelConfigWatcher) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (w *secretBackendModelConfigWatcher) processModelChange() (bool, error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	latest, err := w.backendGetter.GetSecretBackendID(ctx)
	if err != nil {
		return false, errors.Trace(err)
	}
	if w.currentSecretBackend == latest {
		return false, nil
	}
	w.logger.Tracef("secret backend was changed from %s to %s", w.currentSecretBackend, latest)
	w.currentSecretBackend = latest
	return true, nil
}
