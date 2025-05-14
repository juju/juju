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

	// GetDomainServices is used to extract the domain services from the
	// dependency getter.
	GetDomainServices func(getter dependency.Getter, name string) (DomainServices, error)

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

			domainServices, err := config.GetDomainServices(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Capture(err)
			}

			controllerConfigService := domainServices.ControllerConfig()
			applicationService := domainServices.Application()
			controllerNodeService := domainServices.ControllerNode()
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

// GetDomainServices extracts the domain services from the input dependency
// getter, then returns the domain services from it.
func GetDomainServices(getter dependency.Getter, name string) (DomainServices, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.DomainServices) DomainServices {
		return newDomainServicesShim(factory)
	})
}

// DomainServices is a subset of the services.DomainServices interface that
// is implemented by the DomainServicesShim.
type DomainServices interface {
	Network() NetworkService
	ControllerConfig() ControllerConfigService
	Application() ApplicationService
	ControllerNode() ControllerNodeService
}

func newDomainServicesShim(factory services.DomainServices) DomainServicesShim {
	return DomainServicesShim{factory}
}

// DomainServicesShim is a shim that implements the DomainServices interface.
type DomainServicesShim struct {
	factory services.DomainServices
}

// Network returns the network service.
func (d DomainServicesShim) Network() NetworkService {
	return d.factory.Network()
}

// ControllerConfig returns the controller config service.
func (d DomainServicesShim) ControllerConfig() ControllerConfigService {
	return d.factory.ControllerConfig()
}

// Application returns the application service.
func (d DomainServicesShim) Application() ApplicationService {
	return d.factory.Application()
}

// ControllerNode returns the controller node service.
func (d DomainServicesShim) ControllerNode() ControllerNodeService {
	return d.factory.ControllerNode()
}
