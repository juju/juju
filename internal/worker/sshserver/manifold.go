// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/virtualhostname"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/internal/services"
)

// GetControllerConfigServiceFunc is a helper function that gets
// a controller config service from the manifold.
type GetControllerConfigServiceFunc = func(getter dependency.Getter, name string) (ControllerConfigService, error)

// GetControllerSSHHostKeyServiceFunc is a helper function that gets the
// controller SSH host key service from the manifold.
type GetControllerSSHHostKeyServiceFunc = func(getter dependency.Getter, name string) (ControllerSSHHostKeyService, error)

// GetDomainServicesGetterFunc is a helper function that gets the model domain
// services getter from the manifold.
type GetDomainServicesGetterFunc = func(getter dependency.Getter, name string) (services.DomainServicesGetter, error)

// GetSSHServiceFunc is a helper function that gets the model SSH service from
// the manifold.
type GetSSHServiceFunc = func(context.Context, services.DomainServicesGetter, model.UUID) (SSHModelService, error)

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}

// GetControllerSSHHostKeyService gets the controller SSH host key service from
// the controller domain services dependency.
func GetControllerSSHHostKeyService(getter dependency.Getter, name string) (ControllerSSHHostKeyService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ControllerSSHHostKeyService {
		return factory.SSHServerHostKey()
	})
}

// GetDomainServicesGetter gets the model domain services getter from the
// domain services worker dependency.
func GetDomainServicesGetter(getter dependency.Getter, name string) (services.DomainServicesGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.DomainServicesGetter) services.DomainServicesGetter {
		return factory
	})

}

// GetSSHService gets the model SSH service from the current model domain
// services dependency.
func GetSSHService(ctx context.Context, domainServicesGetter services.DomainServicesGetter, modelUUID model.UUID) (SSHModelService, error) {
	domainServices, err := domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices.SSH(), nil
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
	// GetControllerSSHHostKeyService is used to get the controller SSH host key
	// service from the manifold.
	GetControllerSSHHostKeyService GetControllerSSHHostKeyServiceFunc
	// GetDomainServicesGetter is used to get the model domain services getter
	// from the manifold.
	GetDomainServicesGetter GetDomainServicesGetterFunc
	// GetSSHService is used to get the SSH service from the manifold.
	GetSSHService GetSSHServiceFunc
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
	if config.GetControllerSSHHostKeyService == nil {
		return errors.NotValidf("nil GetControllerSSHHostKeyService")
	}
	if config.GetDomainServicesGetter == nil {
		return errors.NotValidf("nil GetDomainServicesGetter")
	}
	if config.GetSSHService == nil {
		return errors.NotValidf("nil GetSSHService")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
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
func (config ManifoldConfig) startWrapperWorker(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
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
	controllerSSHHostKeyService, err := config.GetControllerSSHHostKeyService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	domainServicesGetter, err := config.GetDomainServicesGetter(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sshService := sshService{
		controllerSSHHostKeyService: controllerSSHHostKeyService,
		domainServicesGetter:        domainServicesGetter,
		getSSHService:               config.GetSSHService,
	}

	return config.NewServerWrapperWorker(ServerWrapperWorkerConfig{
		ControllerConfigService: controllerConfigService,
		SSHService:              sshService,
		NewServerWorker:         config.NewServerWorker,
		Logger:                  config.Logger,
		SessionHandler:          &stubSessionHandler{},
	})
}

// sshService wraps our ssh domain services to enable two things:
//  1. Direct controller model accesss via the ControllerSSHHostKeyService interface.
//  2. Model-scoped access to the SSHModelService interface which underlying calls "ServicesForModel".
//     We require the SSH server isn't the usual WS approach where the model uuid is populated
//     by the time we reach the service, and instead, we must call the methods WITH the UUID received
//     from the virtual host name.
type sshService struct {
	controllerSSHHostKeyService ControllerSSHHostKeyService
	domainServicesGetter        services.DomainServicesGetter
	getSSHService               GetSSHServiceFunc
}

// SSHServerHostKey returns the controller SSH server host key.
func (s sshService) SSHServerHostKey(ctx context.Context) (string, error) {
	return s.controllerSSHHostKeyService.SSHServerHostKey(ctx)
}

// VirtualHostKey returns the terminating SSH host key for a virtual hostname.
// The virtual hostname contains the model UUID for the destination model database.
func (s sshService) VirtualHostKey(ctx context.Context, info virtualhostname.Info) (string, error) {
	sshService, err := s.getSSHService(ctx, s.domainServicesGetter, info.ModelUUID())
	if err != nil {
		return "", errors.Trace(err)
	}
	return sshService.VirtualHostKey(ctx, info)
}

// InsertSSHConnRequest inserts a new SSH connection request.
// The SSH connection request contains the model UUID for the destination model database.
func (s sshService) InsertSSHConnRequest(ctx context.Context, req domainssh.SSHConnRequest) error {
	sshService, err := s.getSSHService(ctx, s.domainServicesGetter, req.ModelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	return sshService.InsertSSHConnRequest(ctx, req)
}
