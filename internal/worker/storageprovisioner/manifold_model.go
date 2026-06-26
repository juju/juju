// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/storage"
)

// ModelManifoldConfig defines a storage provisioner's configuration and dependencies.
type ModelManifoldConfig struct {
	DomainServicesName  string
	StorageRegistryName string

	Clock      clock.Clock
	Model      names.ModelTag
	StorageDir string
	NewWorker  func(config Config) (worker.Worker, error)
	Logger     logger.Logger
}

// Validate returns an error if the config cannot be relied upon to start a worker.
func (config ModelManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.StorageRegistryName == "" {
		return errors.NotValidf("empty StorageRegistryName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// ModelManifold returns a dependency.Manifold that runs a storage provisioner.
func ModelManifold(config ModelManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.DomainServicesName, config.StorageRegistryName},
		Start: func(context context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var domainServices services.DomainServices
			if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
				return nil, errors.Trace(err)
			}
			var registry storage.ProviderRegistry
			if err := getter.Get(config.StorageRegistryName, &registry); err != nil {
				return nil, errors.Trace(err)
			}

			adapter := &modelStorageAdapter{
				storageSvc:     domainServices.StorageProvisioning(),
				machineSvc:     domainServices.Machine(),
				appSvc:         domainServices.Application(),
				removalSvc:     domainServices.Removal(),
				statusSvc:      domainServices.Status(),
				blockDeviceSvc: domainServices.BlockDevice(),
			}

			w, err := config.NewWorker(Config{
				Model:       config.Model,
				Scope:       config.Model,
				StorageDir:  config.StorageDir,
				Volumes:     adapter,
				Filesystems: adapter,
				Life:        adapter,
				Registry:    registry,
				Machines:    adapter,
				Status:      adapter,
				Clock:       config.Clock,
				Logger:      config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
