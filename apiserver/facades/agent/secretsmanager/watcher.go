// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
)

type secretBackendModelConfigWatcher struct {
	catacomb          catacomb.Catacomb
	out               chan struct{}
	src               state.NotifyWatcher
	modelConfigGetter ModelConfigState

	currentSecretBackend string
}

func newSecretBackendModelConfigWatcher(modelConfigGetter ModelConfigState, src state.NotifyWatcher) (*secretBackendModelConfigWatcher, error) {
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
	return w, err
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

func (w *secretBackendModelConfigWatcher) Changes() corewatcher.NotifyChannel {
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
			changed, err := w.isSecretBackendChanged()
			if err != nil {
				return errors.Trace(err)
			}
			if !changed && !sendInitial {
				continue
			}
			out = w.out
		case out <- struct{}{}:
			out = nil
			sendInitial = false
		}
	}
}

func (w *secretBackendModelConfigWatcher) isSecretBackendChanged() (bool, error) {
	modelConfig, err := w.modelConfigGetter.ModelConfig()
	if err != nil {
		return false, errors.Trace(err)
	}
	latest := modelConfig.SecretBackend()
	if w.currentSecretBackend == latest {
		return false, nil
	}
	w.currentSecretBackend = latest
	return true, nil
}
