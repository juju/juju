// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/internal/services"
)

// GetControllerConfigServiceFunc is a helper function that gets
// a controller config service from the manifold.
type GetControllerConfigServiceFunc = func(getter dependency.Getter, name string) (ControllerConfigService, error)

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}

// ManifoldConfig holds the information necessary to run an embedded SSH server
// worker in a dependency.Engine.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain services worker.
	DomainServicesName string
	// NewServerWrapperWorker is the function that creates the embedded SSH server worker.
	NewServerWrapperWorker func(ServerWrapperWorkerConfig) (worker.Worker, error)
	// NewServerWorker is the function that creates a worker that has a catacomb
	// to run the server and other worker dependencies.
	NewServerWorker func(ServerWorkerConfig) (worker.Worker, error)
	// GetControllerConfigService is used to get a service from the manifold.
	GetControllerConfigService GetControllerConfigServiceFunc
	// NewSSHServerListener is the function that creates a listener, based on
	// an existing listener for the server worker.
	NewSSHServerListener func(net.Listener, time.Duration) net.Listener
	// Logger is the logger to use for the worker.
	Logger logger.Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.NewServerWrapperWorker == nil {
		return errors.NotValidf("nil NewServerWrapperWorker")
	}
	if config.NewServerWorker == nil {
		return errors.NotValidf("nil NewServerWorker")
	}
	if config.GetControllerConfigService == nil {
		return errors.NotValidf("nil GetControllerConfigService")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewSSHServerListener == nil {
		return errors.NotValidf("nil NewSSHServerListener")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an embedded SSH server
// worker. The manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: config.startWrapperWorker,
	}
}

// startWrapperWorker starts the SSH server worker wrapper passing the necessary dependencies.
func (config ManifoldConfig) startWrapperWorker(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
	// ssh jump server is not enabled by default, but it must be enabled
	// via a feature flag.
	if !featureflag.Enabled(featureflag.SSHJump) {
		config.Logger.Debugf(context.Background(), "SSH jump server worker is not enabled.")
		return nil, dependency.ErrUninstall
	}
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigService, err := config.GetControllerConfigService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewServerWrapperWorker(ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		NewServerWorker:         config.NewServerWorker,
		Logger:                  config.Logger,
		NewSSHServerListener:    config.NewSSHServerListener,
		SessionHandler:          &stubSessionHandler{},
	})
}

// NewSSHServerListener returns a listener based on the given listener.
func NewSSHServerListener(l net.Listener, t time.Duration) net.Listener {
	return l
}
