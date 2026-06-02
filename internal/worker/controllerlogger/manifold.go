// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlogger

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	environsconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/services"
)

// ModelConfigService provides access to model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current model configuration.
	ModelConfig(ctx context.Context) (*environsconfig.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch(ctx context.Context) (watcher.StringsWatcher, error)
}

// ModelService provides access to the controller model UUID.
type ModelService interface {
	// GetControllerModelUUID returns the UUID of the controller model.
	GetControllerModelUUID(ctx context.Context) (coremodel.UUID, error)
}

// ManifoldConfig defines the configuration for a controller-only logging
// worker manifold.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain-services dependency.
	DomainServicesName string

	// LoggerContext is the logger context used by the worker.
	LoggerContext corelogger.LoggerContext

	// Logger is the logger used by the worker for its own messages.
	Logger corelogger.Logger

	// Tag is the controller agent tag.
	Tag names.Tag

	// LoggingOverride is the persisted logging override from the controller
	// agent config.
	LoggingOverride string

	// UpdateAgentFunc persists the current logging config.
	UpdateAgentFunc func(string) error
}

// Validate checks that all required configuration fields are set.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.LoggerContext == nil {
		return errors.NotValidf("nil LoggerContext")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Tag == nil {
		return errors.NotValidf("nil Tag")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a controller-only logging
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	modelService, err := getControllerDomainServices(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerModelUUID, err := modelService.GetControllerModelUUID(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelConfigService, err := getModelConfigService(ctx, getter, config.DomainServicesName, controllerModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	workerConfig := Config{
		Context:         config.LoggerContext,
		ModelConfigSvc:  modelConfigService,
		Tag:             config.Tag,
		Logger:          config.Logger,
		Override:        config.LoggingOverride,
		UpdateAgentFunc: config.UpdateAgentFunc,
	}
	return NewWorker(ctx, workerConfig)
}

// getControllerDomainServices retrieves the model service from the controller
// domain services via the dependency getter.
func getControllerDomainServices(getter dependency.Getter, name string) (ModelService, error) {
	var controllerServices services.ControllerDomainServices
	if err := getter.Get(name, &controllerServices); err != nil {
		return nil, errors.Trace(err)
	}
	return controllerServices.Model(), nil
}

// getModelConfigService retrieves the model config service for the controller
// model via the domain services getter.
func getModelConfigService(
	ctx context.Context,
	getter dependency.Getter,
	name string,
	controllerModelUUID coremodel.UUID,
) (ModelConfigService, error) {
	var domainServicesGetter services.DomainServicesGetter
	if err := getter.Get(name, &domainServicesGetter); err != nil {
		return nil, errors.Trace(err)
	}
	ds, err := domainServicesGetter.ServicesForModel(ctx, controllerModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ds.Config(), nil
}
