// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modellife

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent/engine"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

// ModelService is an interface that provides access to model
// information, specifically the life of a model.
type ModelService interface {
	// GetModelLife returns the life associated with the provided uuid.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	// - [modelerrors.NotActivated]: When the model has not been activated.
	GetModelLife(ctx context.Context, uuid model.UUID) (life.Value, error)

	// WatchModel returns a watcher that emits an event if the model changes.
	WatchModel(ctx context.Context, modelUUID model.UUID) (watcher.NotifyWatcher, error)
}

// ManifoldConfig describes how to configure and construct a Worker,
// and what registered resources it may depend upon.
type ManifoldConfig struct {
	DomainServicesName string

	ModelUUID model.UUID

	NewWorker       func(context.Context, Config) (worker.Worker, error)
	GetModelService func(dependency.Getter, string) (ModelService, error)
}

func (c ManifoldConfig) Validate() error {
	if c.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if c.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if c.GetModelService == nil {
		return errors.NotValidf("nil GetModelService")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a Worker as
// configured.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Output: engine.FlagOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			modelService, err := config.GetModelService(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			worker, err := config.NewWorker(ctx, Config{
				ModelService: modelService,
				ModelUUID:    config.ModelUUID,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return worker, nil
		},
		Filter: internalworker.ShouldWorkerUninstall,
	}
}

// GetModelService retrieves the ModelService from the dependency getter
// using the provided name. It returns an error if the dependency cannot
// be found or if the type assertion fails.
func GetModelService(getter dependency.Getter, name string) (ModelService, error) {
	return coredependency.GetDependencyByName(getter, name, func(a services.ControllerDomainServices) ModelService {
		return a.Model()
	})
}
