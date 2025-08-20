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
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	domainmodel "github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/services"
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
type NewAPIInfoGetterFunc func(DomainServicesGetter, logger.Logger) APIInfoGetter

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
	// ModelService returns the ModelService for the domain.
	Model() ModelService
	// ControllerConfig returns the ControllerConfigService for the domain.
	ControllerConfig() ControllerConfigService
	// ControllerNodeService returns the ControllerNodeService for the domain.
	ControllerNode() ControllerNodeService
}

// ExternalControllerService is an interface that provides methods to interact
// with the external controller service.
type ExternalControllerService interface {
	// ControllerForModel returns the controller record that's associated
	// with the modelUUID.
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)
	// UpdateExternalController updates the external controller information.
	UpdateExternalController(context.Context, crossmodel.ControllerInfo) error
}

// ModelService is an interface that provides methods to interact with the model
// service.
type ModelService interface {
	// CheckModelExists checks if a model exists within the controller.
	CheckModelExists(ctx context.Context, modelUUID model.UUID) (bool, error)
	// ModelRedirection returns redirection information for the current model.
	ModelRedirection(ctx context.Context, modelUUID model.UUID) (domainmodel.ModelRedirection, error)
}

// ControllerConfigService is an interface that provides methods to interact
// with the controller configuration service.
type ControllerConfigService interface {
	// ControllerConfig returns the controller configuration for the model.
	ControllerConfig(ctx context.Context) (controller.Config, error)
}

// ControllerNodeService represents a way to get controller api addresses.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgents returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
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
				APIInfoGetter:    config.NewAPIInfoGetter(domainServicesGetter, config.Logger),
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

// GetDomainServicesGetter returns a function that retrieves a
// DomainServicesGetter for the Worker.
func GetDomainServicesGetter(getter dependency.Getter, name string) (DomainServicesGetter, error) {
	var g services.DomainServicesGetter
	if err := getter.Get(name, &g); err != nil {
		return nil, errors.Trace(err)
	}

	return domainServicesGetter{
		domainServicesGetter: g,
	}, nil
}

type domainServicesGetter struct {
	domainServicesGetter services.DomainServicesGetter
}

// ServicesForModel returns a DomainServices for the specified model.
func (d domainServicesGetter) ServicesForModel(ctx context.Context, modelUUID model.UUID) (DomainServices, error) {
	services, err := d.domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices{
		services: services,
	}, nil
}

type domainServices struct {
	services services.DomainServices
}

// ExternalController returns the ExternalControllerService for the domain.
func (d domainServices) ExternalController() ExternalControllerService {
	return d.services.ExternalController()
}

// Model returns the ModelService for the domain.
func (d domainServices) Model() ModelService {
	return d.services.Model()
}

// ControllerConfig returns the ControllerConfigService for the domain.
func (d domainServices) ControllerConfig() ControllerConfigService {
	return d.services.ControllerConfig()
}

// ControllerNode returns the ControllerNodeService for the domain.
func (d domainServices) ControllerNode() ControllerNodeService {
	return d.services.ControllerNode()
}

var (
	// connectionTag is the user tag used for connections created by this
	// worker. It uses the anonymous username to ensure that it does not conflict
	// with any other user tags.
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

type apiInfoGetter struct {
	domainServicesGetter DomainServicesGetter
	logger               logger.Logger
}

// NewAPIInfoGetter creates a new APIInfoGetter that retrieves API information
// for models using the provided DomainServicesGetter.
func NewAPIInfoGetter(getter DomainServicesGetter, logger logger.Logger) APIInfoGetter {
	return &apiInfoGetter{
		domainServicesGetter: getter,
		logger:               logger,
	}
}

// GetAPIInfoForModel retrieves the API information for the specified model.
func (a *apiInfoGetter) GetAPIInfoForModel(ctx context.Context, modelUUID model.UUID) (api.Info, error) {
	services, err := a.domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return api.Info{}, errors.Trace(err)
	}

	// See if the model exists locally first, before attempting to check
	// if the model is hosted on an external controller.
	exists, err := services.Model().CheckModelExists(ctx, modelUUID)
	if err != nil {
		return api.Info{}, errors.Trace(err)
	} else if exists {
		return a.getAPIInfoFromLocalModel(ctx, modelUUID, services)
	}

	// The model doesn't exist locally, so we now need to check if it exists
	// on an external controller.
	return a.getAPIInfoForExternalController(ctx, modelUUID, services)
}

func (a *apiInfoGetter) getAPIInfoFromLocalModel(ctx context.Context, modelUUID model.UUID, services DomainServices) (api.Info, error) {
	controllerConfig, err := services.ControllerConfig().ControllerConfig(ctx)
	if err != nil {
		return api.Info{}, errors.Trace(err)
	}

	addrs, err := services.ControllerNode().GetAllAPIAddressesForAgents(ctx)
	if err != nil {
		return api.Info{}, errors.Trace(err)
	}

	caCert, _ := controllerConfig.CACert()
	return api.Info{
		ModelTag: names.NewModelTag(modelUUID.String()),
		Addrs:    addrs,
		CACert:   caCert,
	}, nil
}

func (a *apiInfoGetter) getAPIInfoForExternalController(ctx context.Context, modelUUID model.UUID, services DomainServices) (api.Info, error) {
	externalControllerService := services.ExternalController()

	controllerInfo, err := externalControllerService.ControllerForModel(ctx, modelUUID.String())
	if err != nil && !errors.Is(err, modelerrors.NotFound) {
		return api.Info{}, errors.Trace(err)
	} else if err == nil {
		return api.Info{
			ModelTag: names.NewModelTag(modelUUID.String()),
			Addrs:    controllerInfo.Addrs,
			CACert:   controllerInfo.CACert,
		}, nil
	}

	// The model may have been migrated from this controller to another.
	// If so, save the target as an external controller.
	// This will preserve cross-model relation consumers for models that were
	// on the same controller as migrated model, but not for consumers on other
	// controllers.
	// They will have to follow redirects and update their own relation data.
	modelRedirection, err := services.Model().ModelRedirection(ctx, modelUUID)
	if errors.Is(err, modelerrors.ModelNotRedirected) {
		return api.Info{}, modelerrors.NotFound
	} else if err != nil {
		return api.Info{}, errors.Trace(err)
	}

	a.logger.Debugf(ctx, "found migrated model on another controller, saving the information")
	err = externalControllerService.UpdateExternalController(ctx, crossmodel.ControllerInfo{
		ControllerUUID: modelRedirection.ControllerUUID,
		Alias:          modelRedirection.ControllerAlias,
		Addrs:          modelRedirection.Addresses,
		CACert:         modelRedirection.CACert,
		ModelUUIDs:     []string{modelUUID.String()},
	})
	if err != nil {
		return api.Info{}, errors.Trace(err)
	}

	return api.Info{
		ModelTag: names.NewModelTag(modelUUID.String()),
		Addrs:    modelRedirection.Addresses,
		CACert:   modelRedirection.CACert,
	}, nil
}
