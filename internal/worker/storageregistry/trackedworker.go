// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageregistry

import (
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/storage"
)

type trackedWorker struct {
	tomb tomb.Tomb

	provider storage.ProviderRegistry
}

// NewTrackedWorker creates a new tracked worker for a storage provider
// registry.
func NewTrackedWorker(reg storage.ProviderRegistry) (worker.Worker, error) {
	w := &trackedWorker{
		provider: reg,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *trackedWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *trackedWorker) Wait() error {
	return w.tomb.Wait()
}

// StorageProviderTypes returns the storage provider types contained within this
// registry.
//
// Determining the supported storage providers may be dynamic. Multiple calls
// for the same registry must return consistent results.
func (w *trackedWorker) StorageProviderTypes() ([]storage.ProviderType, error) {
	return w.provider.StorageProviderTypes()
}

// StorageProvider returns the storage provider with the given provider type.
// StorageProvider must return an errors satisfying errors.IsNotFound if the
// registry does not contain the specified provider type.
func (w *trackedWorker) StorageProvider(providerType storage.ProviderType) (storage.Provider, error) {
	return w.provider.StorageProvider(providerType)
}

func (w *trackedWorker) loop() error {
	select {
	case <-w.tomb.Dying():
		return w.tomb.Err()
	}
}
