// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/state"
)

type secretBackendModelConfigWatcher struct {
	catacomb          catacomb.Catacomb
	out               chan struct{}
	src               state.NotifyWatcher
	modelConfigGetter Model

	currentSecretBackend string
}

func newSecretBackendModelConfigWatcher(modelConfigGetter Model, src state.NotifyWatcher) (state.NotifyWatcher, error) {
	w := &secretBackendModelConfigWatcher{
		out:               make(chan struct{}),
		src:               src,
		modelConfigGetter: modelConfigGetter,
	}
	modelConfig, err := w.modelConfigGetter.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.currentSecretBackend = modelConfig.SecretBackend()

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

func (w *secretBackendModelConfigWatcher) processModelChange() (bool, error) {
	modelConfig, err := w.modelConfigGetter.ModelConfig()
	if err != nil {
		return false, errors.Trace(err)
	}
	latest := modelConfig.SecretBackend()
	if w.currentSecretBackend == latest {
		return false, nil
	}
	logger.Tracef("secret backend was changed from %s to %s", w.currentSecretBackend, latest)
	w.currentSecretBackend = latest
	return true, nil
}
