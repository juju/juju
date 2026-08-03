// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
)

// GetControllerNodeServiceFunc is a helper function that gets the controller
// node service from a controller domain services dependency.
type GetControllerNodeServiceFunc = func(getter dependency.Getter, name string) (ControllerNodeService, error)

// GetDomainServicesGetterFunc is a helper function that gets the domain
// services getter from the manifold.
type GetDomainServicesGetterFunc = func(getter dependency.Getter, name string) (services.DomainServicesGetter, error)

// GetControllerNodeService gets the controller node service from the
// controller domain services dependency.
func GetControllerNodeService(getter dependency.Getter, name string) (ControllerNodeService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ControllerNodeService {
		return factory.ControllerNode()
	})
}

// GetDomainServicesGetter gets the domain services getter from the domain
// services worker dependency.
func GetDomainServicesGetter(getter dependency.Getter, name string) (services.DomainServicesGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.DomainServicesGetter) services.DomainServicesGetter {
		return factory
	})
}

// GetSSHService gets the model SSH service from the current model domain
// services dependency.
func GetSSHService(ctx context.Context, domainServicesGetter services.DomainServicesGetter, modelUUID coremodel.UUID) (SSHModelService, error) {
	domainServices, err := domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices.SSH(), nil
}

// GetMachineService gets the model machine service from the current model
// domain services dependency.
func GetMachineService(ctx context.Context, domainServicesGetter services.DomainServicesGetter, modelUUID coremodel.UUID) (MachineService, error) {
	domainServices, err := domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices.Machine(), nil
}

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain services worker.
	DomainServicesName string
	// Clock used by the tunnel tracker.
	Clock clock.Clock
	// GetControllerNodeService is used to get the controller node service from
	// the manifold.
	GetControllerNodeService GetControllerNodeServiceFunc
	// GetDomainServicesGetter is used to get the domain services getter from
	// the manifold.
	GetDomainServicesGetter GetDomainServicesGetterFunc
	// GetSSHService is used to get the model SSH service from the manifold.
	GetSSHService GetSSHServiceFunc
	// GetMachineService is used to get the model machine service from the
	// manifold.
	GetMachineService GetMachineServiceFunc
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.GetControllerNodeService == nil {
		return errors.NotValidf("nil GetControllerNodeService")
	}
	if config.GetDomainServicesGetter == nil {
		return errors.NotValidf("nil GetDomainServicesGetter")
	}
	if config.GetSSHService == nil {
		return errors.NotValidf("nil GetSSHService")
	}
	if config.GetMachineService == nil {
		return errors.NotValidf("nil GetMachineService")
	}
	return nil
}

// Manifold returns a manifold whose worker contains an SSH tunnel tracker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Output: outputFunc,
		Start:  config.start,
	}
}

func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	controllerNodeService, err := config.GetControllerNodeService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	domainServicesGetter, err := config.GetDomainServicesGetter(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewWorker(domainServicesGetter, config.GetSSHService, config.GetMachineService, controllerNodeService, config.Clock)
}

// outputFunc extracts a tunnel tracker from a sshTunnelerWorker.
func outputFunc(in worker.Worker, out any) error {
	inWorker, _ := in.(*sshTunnelerWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *TunnelTracker:
		*outPointer = inWorker.tunnelTracker
	default:
		return errors.Errorf("out should be *sshtunneler.TunnelTracker; got %T", out)
	}
	return nil
}
