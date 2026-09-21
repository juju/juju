// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupsweeper

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
)

// NewWorkerFunc creates the sweeper worker from the resolved config.
type NewWorkerFunc func(WorkerConfig) (worker.Worker, error)

// ManifoldConfig defines the names of the manifolds on which a Manifold
// will depend.
type ManifoldConfig struct {
	// DomainServicesName is the name of the manifold providing the
	// domain services getter.
	DomainServicesName string

	// ControllerModelUUID is the controller model's UUID.
	ControllerModelUUID string

	Clock     clock.Clock
	Logger    logger.Logger
	NewWorker NewWorkerFunc

	// GetModelConfigService extracts the controller model's config
	// service from the domain services dependency.
	GetModelConfigService func(getter dependency.Getter, name string, controllerModelUUID model.UUID) (ModelConfigService, error)
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.ControllerModelUUID == "" {
		return errors.NotValidf("empty ControllerModelUUID")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if cfg.GetModelConfigService == nil {
		return errors.NotValidf("nil GetModelConfigService")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the backup archive
// sweeper worker, using the resource names defined in the supplied
// config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			modelConfigService, err := config.GetModelConfigService(
				getter, config.DomainServicesName, model.UUID(config.ControllerModelUUID))
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(WorkerConfig{
				ModelConfigService: modelConfigService,
				Clock:              config.Clock,
				Logger:             config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// GetControllerModelConfigService extracts the controller model's config
// service from the domain services dependency.
func GetControllerModelConfigService(getter dependency.Getter, name string, controllerModelUUID model.UUID) (ModelConfigService, error) {
	var servicesGetter services.DomainServicesGetter
	if err := getter.Get(name, &servicesGetter); err != nil {
		return nil, errors.Trace(err)
	}
	domainServices, err := servicesGetter.ServicesForModel(context.Background(), controllerModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices.Config(), nil
}
