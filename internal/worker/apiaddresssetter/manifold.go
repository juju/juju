// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// ControllerDomainServices is an interface that defines the
// controller domain services required by the api address setter.
type ControllerDomainServices interface {
	// ControllerConfig returns the controller configuration service.
	ControllerConfig() ControllerConfigService
	// ControllerNode returns the controller node service.
	ControllerNode() ControllerNodeService
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
	// Application returns the application service.
	Application() ApplicationService
	// Network returns the network service.
	Network() NetworkService
}

// ManifoldConfig contains the configuration passed to this
// worker's manifold when run by the dependency engine.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain service factory dependency.
	DomainServicesName string

	// GetDomainServices is used to extract the domain services from the
	// dependency getter.
	GetDomainServices func(getter dependency.Getter, name string, controllerModelUUID model.UUID) (DomainServices, error)

	// GetControllerDomainServices is used to extract the controller domain
	// services from the dependency getter.
	GetControllerDomainServices func(getter dependency.Getter, name string) (ControllerDomainServices, error)

	// NewWorker creates and returns a apiaddressetter worker.
	NewWorker func(Config) (worker.Worker, error)

	// Logger logs stuff.
	Logger logger.Logger
}

// Validate ensures that the configuration is
// correctly populated for manifold operation.
func (config ManifoldConfig) Validate() error {
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
	return nil
}

// Manifold returns a dependency.Manifold that will run the api address setter
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
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

			controllerConfigService := controllerDomainServices.ControllerConfig()
			controllerNodeService := controllerDomainServices.ControllerNode()
			applicationService := domainServices.Application()
			networkService := domainServices.Network()

			controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
			if err != nil {
				return nil, errors.Capture(err)
			}

			w, err := config.NewWorker(Config{
				ControllerConfigService: controllerConfigService,
				ApplicationService:      applicationService,
				ControllerNodeService:   controllerNodeService,
				NetworkService:          networkService,
				APIPort:                 controllerConfig.APIPort(),
				Logger:                  config.Logger,
			})
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
		applicationService: services.Application(),
		networkService:     services.Network(),
	}, nil
}

type domainServices struct {
	applicationService ApplicationService
	networkService     NetworkService
}

// Application returns the application service.
func (s domainServices) Application() ApplicationService {
	return s.applicationService
}

// Network returns the network service.
func (s domainServices) Network() NetworkService {
	return s.networkService
}

// GetControllerDomainServices retrieves the controller domain services
// from the dependency getter.
func GetControllerDomainServices(getter dependency.Getter, name string) (ControllerDomainServices, error) {
	return coredependency.GetDependencyByName(getter, name, func(s services.ControllerDomainServices) ControllerDomainServices {
		return controllerDomainServices{
			controllerConfigService: s.ControllerConfig(),
			controllerNodeService:   s.ControllerNode(),
			modelService:            s.Model(),
		}
	})
}

type controllerDomainServices struct {
	controllerConfigService ControllerConfigService
	controllerNodeService   ControllerNodeService
	modelService            ModelService
}

// ControllerConfig returns the controller configuration service.
func (s controllerDomainServices) ControllerConfig() ControllerConfigService {
	return s.controllerConfigService
}

// ControllerNode returns the controller node service.
func (s controllerDomainServices) ControllerNode() ControllerNodeService {
	return s.controllerNodeService
}

// Model returns the model service.
func (s controllerDomainServices) Model() ModelService {
	return s.modelService
}
