// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremoterelationcaller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/worker/apicaller"
)

// APIRemoteCallerGetter is an interface that provides a method to get the
// remote API caller for a given model.
type APIRemoteCallerGetter interface {
	// GetConnectionForModel returns the remote API connection for the
	// specified model. The connection must be valid for the lifetime of the
	// returned RemoteConnection.
	GetConnectionForModel(ctx context.Context, modelUUID model.UUID) (api.Connection, error)
}

// NewWorkerFunc defines a function that creates a new Worker.
type NewWorkerFunc func(Config) (worker.Worker, error)

// NewAPIInfoGetterFunc defines a function that creates a new APIInfoGetter.
type NewAPIInfoGetterFunc func(DomainServicesGetter) APIInfoGetter

// NewConnectionGetterFunc defines a function that creates a new
// ConnectionGetter.
type NewConnectionGetterFunc func(DomainServicesGetter, logger.Logger) ConnectionGetter

// GetDomainServiceGetterFunc defines a function that retrieves a
// DomainServicesGetter for the Worker.
type GetDomainServiceGetterFunc func(dependency.Getter, string) (DomainServicesGetter, error)

// DomainServicesGetter is an interface that provides a method to get
// a DomainServicesGetter by name.
type DomainServicesGetter interface {
	// ServicesForModel returns a DomainServicesGetter for the specified model.
	ServicesForModel(ctx context.Context, modelUUID model.UUID) (DomainServices, error)
}

// DomainServices is an interface that provides methods to get
// various domain services.
type DomainServices interface {
	// ExternalController returns the ExternalControllerService for the domain.
	ExternalController() ExternalControllerService
}

// ExternalControllerService is an interface that provides methods to
// interact with the external controller service.
type ExternalControllerService interface {
	// UpdateExternalController updates the external controller information.
	UpdateExternalController(context.Context, crossmodel.ControllerInfo) error
}

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	DomainServicesName string

	NewWorker                   NewWorkerFunc
	NewAPIInfoGetter            NewAPIInfoGetterFunc
	NewConnectionGetter         NewConnectionGetterFunc
	GetDomainServicesGetterFunc GetDomainServiceGetterFunc
	Logger                      logger.Logger
	Clock                       clock.Clock
}

func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.NewAPIInfoGetter == nil {
		return errors.NotValidf("nil NewAPIInfoGetter")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.GetDomainServicesGetterFunc == nil {
		return errors.NotValidf("nil GetDomainServicesGetterFunc")
	}
	if config.NewConnectionGetter == nil {
		return errors.NotValidf("nil NewConnectionGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			domainServicesGetter, err := config.GetDomainServicesGetterFunc(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				APIInfoGetter:    config.NewAPIInfoGetter(domainServicesGetter),
				ConnectionGetter: config.NewConnectionGetter(domainServicesGetter, config.Logger),
				Clock:            config.Clock,
				Logger:           config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
		Output: remoteOutput,
	}
}

func remoteOutput(in worker.Worker, out any) error {
	w, ok := in.(*remoteWorker)
	if !ok {
		return errors.NotValidf("expected remoteWorker, got %T", in)
	}

	switch out := out.(type) {
	case *APIRemoteCallerGetter:
		*out = w
	default:
		return errors.NotValidf("expected *api.Connection, got %T", out)
	}
	return nil
}

var (
	connectionTag = names.NewUserTag(api.AnonymousUsername)
)

type connectionGetter struct {
	domainServicesGetter DomainServicesGetter
	newConnection        func(ctx context.Context, apiInfo *api.Info) (api.Connection, error)
	logger               logger.Logger
}

// NewConnectionGetter creates a new ConnectionGetter that retrieves connections
// for models using the provided DomainServicesGetter to update external
// controller information.
func NewConnectionGetter(getter DomainServicesGetter, logger logger.Logger) ConnectionGetter {
	return &connectionGetter{
		domainServicesGetter: getter,
		newConnection:        apicaller.NewExternalControllerConnection,
		logger:               logger,
	}
}

// GetConnectionForModel returns the remote API connection for the
// specified model. The connection must be valid for the lifetime of the
// returned RemoteConnection.
func (c connectionGetter) GetConnectionForModel(ctx context.Context, modelUUID model.UUID, apiInfo api.Info) (api.Connection, error) {
	info := &apiInfo
	info.Tag = connectionTag
	conn, err := c.newConnection(ctx, info)
	if err == nil {
		return conn, nil
	}

	var redirectErr *api.RedirectError

	// This is isn't a redirect error, so we return the error as is.
	if !errors.As(errors.Cause(err), &redirectErr) {
		return nil, errors.Trace(err)
	}

	// If we got a redirect error, we need to create a new connection with the
	// redirected API info.
	redirectedInfo := &apiInfo
	redirectedInfo.Tag = connectionTag
	redirectedInfo.Addrs = network.CollapseToHostPorts(redirectErr.Servers).Strings()
	redirectedInfo.CACert = redirectErr.CACert

	conn, err = c.newConnection(ctx, redirectedInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We got a new connection from the redirect, update the local
	// external controller information, so we don't have to perform the
	// redirect again in the future. If there is a failure to update the
	// external controller, we log it but do not return an error, as the
	// connection is still valid. We will just retry any time we reopen the
	// connection.
	services, err := c.domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		c.logger.Infof(ctx, "failed to get domain services for model %s: %v", modelUUID, err)
		return conn, nil
	}

	controllerInfo := crossmodel.ControllerInfo{
		ControllerUUID: redirectErr.ControllerTag.Id(),
		Alias:          redirectErr.ControllerAlias,
		Addrs:          redirectedInfo.Addrs,
		CACert:         redirectedInfo.CACert,
	}

	externalControllerServices := services.ExternalController()
	if err := externalControllerServices.UpdateExternalController(ctx, controllerInfo); err != nil {
		c.logger.Infof(ctx, "failed to update external controller for model %s: %v", modelUUID, err)
		return conn, nil
	}

	return conn, nil
}
