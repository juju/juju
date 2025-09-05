// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

// NewWorkerFunc function that allows the creation of ChangeStreamPruner.
type NewWorkerFunc func(WorkerConfig) (worker.Worker, error)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	DomainServiceName string

	// GetChangeStreamService is used to extract the removal
	// service from domain service dependency.
	GetChangeStreamService func(getter dependency.Getter, name string) (ChangeStreamService, error)

	Clock     clock.Clock
	Logger    logger.Logger
	NewWorker NewWorkerFunc
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServiceName == "" {
		return errors.NotValidf("empty DomainServiceName")
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
	if cfg.GetChangeStreamService == nil {
		return errors.NotValidf("nil GetChangeStreamService")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the changestream
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServiceName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			changeStreamService, err := config.GetChangeStreamService(getter, config.DomainServiceName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(WorkerConfig{
				ChangeStreamService: changeStreamService,
				Clock:               config.Clock,
				Logger:              config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// GetControllerChangeStreamService extracts the controller change stream
// service from the domain service dependency.
func GetControllerChangeStreamService(getter dependency.Getter, name string) (ChangeStreamService, error) {
	var services services.ControllerDomainServices
	if err := getter.Get(name, &services); err != nil {
		return nil, errors.Trace(err)
	}
	return services.ControllerChangeStream(), nil
}

// GetModelChangeStreamService extracts the model change stream service from the
// domain service dependency.
func GetModelChangeStreamService(getter dependency.Getter, name string) (ChangeStreamService, error) {
	var services services.ModelDomainServices
	if err := getter.Get(name, &services); err != nil {
		return nil, errors.Trace(err)
	}
	return services.ChangeStream(), nil
}
