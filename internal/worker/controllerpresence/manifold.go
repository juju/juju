// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerpresence

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/apiremotecaller"
)

// ControllerDomainServices is an interface that defines the
// controller domain services required by the api address setter.
type ControllerDomainServices interface {
	// Model returns the model service.
	Model() ModelService
}

// ModelService is the interface that the worker uses to get model information.
type ModelService interface {
	// GetControllerModelUUID returns the model uuid for the controller model.
	// If no controller model exists then an error satisfying
	// [modelerrors.NotFound] is returned.
	GetControllerModelUUID(context.Context) (model.UUID, error)
}

// DomainServices is an interface that defines the domain services required by
// the api address setter.
type DomainServices interface {
	// Status returns the status service.
	Status() StatusService
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	APIRemoteCallerName string
	DomainServicesName  string

	// GetDomainServices is used to extract the domain services from the
	// dependency getter.
	GetDomainServices func(getter dependency.Getter, name string, controllerModelUUID model.UUID) (DomainServices, error)

	// GetControllerDomainServices is used to extract the controller domain
	// services from the dependency getter.
	GetControllerDomainServices func(getter dependency.Getter, name string) (ControllerDomainServices, error)

	// NewWorker is the function that creates the worker.
	NewWorker func(WorkerConfig) (worker.Worker, error)

	Logger logger.Logger
	Clock  clock.Clock
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.APIRemoteCallerName == "" {
		return errors.New("empty APIRemoteCallerName not valid").Add(coreerrors.NotValid)
	}
	if config.DomainServicesName == "" {
		return errors.New("empty DomainServicesName not valid").Add(coreerrors.NotValid)
	}
	if config.GetDomainServices == nil {
		return errors.New("nil GetDomainServices not valid").Add(coreerrors.NotValid)
	}
	if config.GetControllerDomainServices == nil {
		return errors.New("nil GetControllerDomainServices not valid").Add(coreerrors.NotValid)
	}
	if config.NewWorker == nil {
		return errors.New("nil NewWorker not valid").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	if config.Clock == nil {
		return errors.New("nil Clock not valid").Add(coreerrors.NotValid)
	}
	return nil
}

// Manifold returns a dependency manifold that runs an API remote caller worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APIRemoteCallerName,
			config.DomainServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Capture(err)
			}

			controllerDomainServices, err := config.GetControllerDomainServices(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Capture(err)
			}

			controllerModelUUID, err := controllerDomainServices.Model().GetControllerModelUUID(ctx)
			if err != nil {
				return nil, errors.Capture(err)
			}

			domainServices, err := config.GetDomainServices(getter, config.DomainServicesName, controllerModelUUID)
			if err != nil {
				return nil, errors.Capture(err)
			}

			var apiRemoteSubscriber apiremotecaller.APIRemoteSubscriber
			if err := getter.Get(config.APIRemoteCallerName, &apiRemoteSubscriber); err != nil {
				return nil, errors.Capture(err)
			}

			cfg := WorkerConfig{
				APIRemoteSubscriber: apiRemoteSubscriber,
				StatusService:       domainServices.Status(),
				Logger:              config.Logger,
				Clock:               config.Clock,
			}

			w, err := config.NewWorker(cfg)
			if err != nil {
				return nil, errors.Capture(err)
			}
			return w, nil
		},
	}
}

// GetDomainServices retrieves the domain services from the dependency getter.
func GetDomainServices(getter dependency.Getter, name string, controllerModelUUID model.UUID) (DomainServices, error) {
	domainServicesGetter, err := coredependency.GetDependencyByName(getter, name, func(s services.DomainServicesGetter) services.DomainServicesGetter {
		return s
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	services, err := domainServicesGetter.ServicesForModel(context.Background(), controllerModelUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return domainServices{
		statusService: services.Status(),
	}, nil
}

type domainServices struct {
	statusService StatusService
}

// Status returns the status service.
func (s domainServices) Status() StatusService {
	return s.statusService
}

// GetControllerDomainServices retrieves the controller domain services
// from the dependency getter.
func GetControllerDomainServices(getter dependency.Getter, name string) (ControllerDomainServices, error) {
	return coredependency.GetDependencyByName(getter, name, func(s services.ControllerDomainServices) ControllerDomainServices {
		return controllerDomainServices{
			modelService: s.Model(),
		}
	})
}

type controllerDomainServices struct {
	modelService ModelService
}

// Model returns the model service.
func (s controllerDomainServices) Model() ModelService {
	return s.modelService
}
