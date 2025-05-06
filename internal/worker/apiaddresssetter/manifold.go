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
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig contains the configuration passed to this
// worker's manifold when run by the dependency engine.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain service factory dependency.
	DomainServicesName string

	// GetControllerConfigService is used to extract the controller config
	// service from controller service dependency.
	GetControllerConfigService func(getter dependency.Getter, name string) (ControllerConfigService, error)

	// GetApplicationService is used to extract the application service
	// from domain service dependency.
	GetApplicationService func(getter dependency.Getter, name string) (ApplicationService, error)

	// GetControllernodeService is used to extract the controller node service
	// from domain service dependency.
	GetControllerNodeService func(getter dependency.Getter, name string) (ControllerNodeService, error)

	// GetNetworkService is used to extract the network service
	// from domain service dependency.
	GetNetworkService func(getter dependency.Getter, name string) (NetworkService, error)

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
	if config.GetControllerConfigService == nil {
		return errors.New("nil GetControllerConfigService not valid").Add(coreerrors.NotValid)
	}
	if config.GetApplicationService == nil {
		return errors.New("nil GetApplicationService not valid").Add(coreerrors.NotValid)
	}
	if config.GetControllerNodeService == nil {
		return errors.New("nil GetControllerNodeService not valid").Add(coreerrors.NotValid)
	}
	if config.GetNetworkService == nil {
		return errors.New("nil GetNetworkService not valid").Add(coreerrors.NotValid)
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

			controllerConfigService, err := config.GetControllerConfigService(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Capture(err)
			}
			applicationService, err := config.GetApplicationService(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Capture(err)
			}
			controllerNodeService, err := config.GetControllerNodeService(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Capture(err)
			}
			networkService, err := config.GetNetworkService(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Capture(err)
			}

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
				ControllerAPIPort:       controllerConfig.ControllerAPIPort(),
				Logger:                  config.Logger,
			})
			if err != nil {
				return nil, errors.Capture(err)
			}
			return w, nil
		},
	}
}

// GetControllerConfigService extracts the controller config service from the input
// dependency getter, then returns the controller config service from it.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.DomainServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}

// GetApplicationService extracts the application service from the input
// dependency getter, then returns the application service from it.
func GetApplicationService(getter dependency.Getter, name string) (ApplicationService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.DomainServices) ApplicationService {
		return factory.Application()
	})
}

// GetControllerNodeService extracts the controller node service from the input
// dependency getter, then returns the controller node service from it.
func GetControllerNodeService(getter dependency.Getter, name string) (ControllerNodeService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.DomainServices) ControllerNodeService {
		return factory.ControllerNode()
	})
}

// GetNetworkService extracts the network service from the input
// dependency getter, then returns the network service from it.
func GetNetworkService(getter dependency.Getter, name string) (NetworkService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.DomainServices) NetworkService {
		return factory.Network()
	})
}
