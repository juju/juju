// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlogger

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	corelogger "github.com/juju/juju/core/logger"
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

// LoggingOverrideReader returns the current persisted logging override when the
// manifold starts.
type LoggingOverrideReader interface {
	LoggingOverride() (string, error)
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

	// LoggingOverrideReader returns the current persisted logging override from
	// the controller agent config.
	LoggingOverrideReader LoggingOverrideReader

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
	if config.LoggingOverrideReader == nil {
		return errors.NotValidf("nil LoggingOverrideReader")
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

	modelConfigService, err := getModelConfigService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	loggingOverride, err := config.LoggingOverrideReader.LoggingOverride()
	if err != nil {
		return nil, errors.Trace(err)
	}

	workerConfig := Config{
		Context:         config.LoggerContext,
		ModelConfigSvc:  modelConfigService,
		Logger:          config.Logger,
		Override:        loggingOverride,
		UpdateAgentFunc: config.UpdateAgentFunc,
	}
	return NewWorker(ctx, workerConfig)
}

// getModelConfigService retrieves the model config service from the domain
// services.
func getModelConfigService(
	getter dependency.Getter,
	name string,
) (ModelConfigService, error) {
	var domainServices services.DomainServices
	if err := getter.Get(name, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices.Config(), nil
}
