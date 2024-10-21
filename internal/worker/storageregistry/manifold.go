// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageregistry

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/common"
)

// StorageRegistryWorker is the interface for the storage registry worker.
type StorageRegistryWorker interface {
	storage.ProviderRegistry
	worker.Worker
}

// StorageRegistryWorkerFunc is the function signature for creating a new
// storage registry worker.
type StorageRegistryWorkerFunc func(storage.ProviderRegistry) (worker.Worker, error)

// ManifoldConfig defines the configuration for the storage registry manifold.
type ManifoldConfig struct {
	ProviderFactoryName      string
	NewStorageRegistryWorker StorageRegistryWorkerFunc
	Clock                    clock.Clock
	Logger                   logger.Logger
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.ProviderFactoryName == "" {
		return errors.NotValidf("empty ProviderFactoryName")
	}
	if cfg.NewStorageRegistryWorker == nil {
		return errors.NotValidf("nil NewStorageRegistryWorker")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the storage registry worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ProviderFactoryName,
		},
		Output: output,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var providerFactory providertracker.ProviderFactory
			if err := getter.Get(config.ProviderFactoryName, &providerFactory); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				ProviderFactory:          providerFactory,
				NewStorageRegistryWorker: config.NewStorageRegistryWorker,
				Clock:                    config.Clock,
				Logger:                   config.Logger,
			})
			return w, errors.Trace(err)
		},
	}
}

func output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*storageRegistryWorker)
	if !ok {
		return errors.Errorf("expected input of storageRegistryWorker, got %T", in)
	}

	switch out := out.(type) {
	case *corestorage.StorageRegistryGetter:
		var target corestorage.StorageRegistryGetter = w
		*out = target
	default:
		return errors.Errorf("expected output of StorageRegistryGetter, got %T", out)
	}
	return nil
}
