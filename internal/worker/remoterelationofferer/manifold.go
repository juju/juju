// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationofferer

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
)

// NewWorkerFunc defines the function signature for creating a new Worker.
type NewWorkerFunc func(Config) (worker.Worker, error)

// NewRemoteOffererApplicationWorkerFunc defines the function signature for creating
// a new remote application worker.
type NewRemoteOffererApplicationWorkerFunc func(RemoteOffererWorkerConfig) (ReportableWorker, error)

// GetCrossModelServicesFunc defines the function signature for getting
// cross-model services.
type GetCrossModelServicesFunc func(getter dependency.Getter, domainServicesName string) (CrossModelRelationService, error)

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	ModelUUID          model.UUID
	DomainServicesName string

	GetCrossModelServices GetCrossModelServicesFunc

	NewWorker                         NewWorkerFunc
	NewRemoteOffererApplicationWorker NewRemoteOffererApplicationWorkerFunc

	Logger logger.Logger
	Clock  clock.Clock

	// Active indicates if the worker should be started. This is only here so
	// that we can work on implementing cross-model relations behind a flag,
	// which prevents the dependency engine from starting the worker because
	// of other errors.
	Active bool
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.GetCrossModelServices == nil {
		return errors.NotValidf("nil GetCrossModelServices")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewRemoteOffererApplicationWorker == nil {
		return errors.NotValidf("nil NewRemoteOffererApplicationWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: func(context context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			if !config.Active {
				return nil, dependency.ErrUninstall
			}

			crossModelRelationService, err := config.GetCrossModelServices(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				ModelUUID: config.ModelUUID,

				CrossModelRelationService:         crossModelRelationService,
				NewRemoteOffererApplicationWorker: config.NewRemoteOffererApplicationWorker,

				Clock:  config.Clock,
				Logger: config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// GetCrossModelServices returns the cross-model relation services
// from the dependency engine.
func GetCrossModelServices(getter dependency.Getter, domainServicesName string) (CrossModelRelationService, error) {
	var services services.DomainServices
	if err := getter.Get(domainServicesName, &services); err != nil {
		return nil, errors.Trace(err)
	}

	return services.CrossModelRelation(), nil
}
