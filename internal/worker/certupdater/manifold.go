// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/services"
)

// ControllerDomainServices is an interface that defines the
// controller domain services required by the api address setter.
type ControllerDomainServices interface {
	// ControllerNode returns the controller node service.
	ControllerNode() ControllerNodeService
}

// ManifoldConfig holds the information necessary to run a certupdater
// in a dependency.Engine.
type ManifoldConfig struct {
	AuthorityName               string
	DomainServicesName          string
	GetControllerDomainServices func(getter dependency.Getter, name string) (ControllerDomainServices, error)
	NewWorker                   func(Config) (worker.Worker, error)
	Logger                      logger.Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AuthorityName == "" {
		return errors.New("empty AuthorityName not valid").Add(coreerrors.NotValid)
	}
	if config.DomainServicesName == "" {
		return errors.New("empty DomainServicesName not valid").Add(coreerrors.NotValid)
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

// Manifold returns a dependency.Manifold that will run a pki Authority.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AuthorityName,
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	var authority pki.Authority
	if err := getter.Get(config.AuthorityName, &authority); err != nil {
		return nil, errors.Capture(err)
	}

	controllerDomainServices, err := config.GetControllerDomainServices(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return config.NewWorker(Config{
		Authority:             authority,
		ControllerNodeService: controllerDomainServices.ControllerNode(),
		Logger:                config.Logger,
	})
}

// GetControllerDomainServices retrieves the controller domain services
// from the dependency getter.
func GetControllerDomainServices(getter dependency.Getter, name string) (ControllerDomainServices, error) {
	return coredependency.GetDependencyByName(getter, name, func(s services.ControllerDomainServices) ControllerDomainServices {
		return controllerDomainServices{
			controllerNodeService: s.ControllerNode(),
		}
	})
}

type controllerDomainServices struct {
	controllerNodeService ControllerNodeService
}

// ControllerNode returns the controller node service.
func (s controllerDomainServices) ControllerNode() ControllerNodeService {
	return s.controllerNodeService
}
