// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api/agent/storageprovisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/storage"
)

// ModelManifoldConfig defines a storage provisioner's configuration and dependencies.
type ModelManifoldConfig struct {
	APICallerName       string
	StorageRegistryName string

	Clock      clock.Clock
	Model      names.ModelTag
	StorageDir string
	NewWorker  func(config Config) (worker.Worker, error)
	Logger     logger.Logger
}

// ModelManifold returns a dependency.Manifold that runs a storage provisioner.
func ModelManifold(config ModelManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName, config.StorageRegistryName},
		Start: func(context context.Context, getter dependency.Getter) (worker.Worker, error) {

			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			var registry storage.ProviderRegistry
			if err := getter.Get(config.StorageRegistryName, &registry); err != nil {
				return nil, errors.Trace(err)
			}

			api, err := storageprovisioner.NewClient(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				Model:        config.Model,
				Scope:        config.Model,
				StorageDir:   config.StorageDir,
				Applications: api,
				Volumes:      api,
				Filesystems:  api,
				Life:         api,
				Registry:     registry,
				Machines:     api,
				Status:       api,
				Clock:        config.Clock,
				Logger:       config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
