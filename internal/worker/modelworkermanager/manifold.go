// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	coredependency "github.com/juju/juju/core/dependency"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/servicefactory"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
	Infof(string, ...interface{})
}

// GetProviderServiceFactoryGetterFunc returns a ProviderServiceFactoryGetter
// from the given dependency.Getter.
type GetProviderServiceFactoryGetterFunc func(getter dependency.Getter, name string) (ProviderServiceFactoryGetter, error)

// ManifoldConfig holds the information necessary to run a model worker manager
// in a dependency.Engine.
type ManifoldConfig struct {
	// AgentName is the name of the agent.Agent dependency.
	AgentName string
	// AuthorityName is the name of the pki.Authority dependency.
	AuthorityName string
	// StateName is the name of the workerstate.StateTracker dependency.
	// Deprecated: Migration to service factory.
	StateName string
	// ServiceFactoryName is used to get the controller service factory
	// dependency.
	ServiceFactoryName string
	// ProviderServiceFactoriesName is used to get the provider service factory
	// getter dependency. This exposes a provider service factory for each
	// model upon request.
	ProviderServiceFactoriesName string
	// LogSinkName is the name of the corelogger.ModelLogger dependency.
	LogSinkName string

	// GetProviderServiceFactoryGetter is used to get the provider service
	// factory getter from the dependency engine. This makes testing a lot
	// simpler, as we can expose the interface directly, without the
	// intermediary type.
	GetProviderServiceFactoryGetter GetProviderServiceFactoryGetterFunc

	// GetControllerConfig is used to get the controller config from the
	// controller service.
	GetControllerConfig GetControllerConfigFunc

	// NewWorker is the function that creates the worker.
	NewWorker func(Config) (worker.Worker, error)
	// NewModelWorker is the function that creates the model worker.
	NewModelWorker NewModelWorkerFunc
	// ModelMetrics is the metrics for the model worker.
	ModelMetrics ModelMetrics
	// Logger is the logger for the worker.
	Logger Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.AuthorityName == "" {
		return errors.NotValidf("empty AuthorityName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if config.ProviderServiceFactoriesName == "" {
		return errors.NotValidf("empty ProviderServiceFactoriesName")
	}
	if config.LogSinkName == "" {
		return errors.NotValidf("empty LogSinkName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewModelWorker == nil {
		return errors.NotValidf("nil NewModelWorker")
	}
	if config.ModelMetrics == nil {
		return errors.NotValidf("nil ModelMetrics")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.GetProviderServiceFactoryGetter == nil {
		return errors.NotValidf("nil GetProviderServiceFactoryGetter")
	}
	if config.GetControllerConfig == nil {
		return errors.NotValidf("nil GetControllerConfig")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a model worker manager.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.AuthorityName,
			config.StateName,
			config.LogSinkName,
			config.ServiceFactoryName,
			config.ProviderServiceFactoriesName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var authority pki.Authority
	if err := getter.Get(config.AuthorityName, &authority); err != nil {
		return nil, errors.Trace(err)
	}

	var logSink corelogger.ModelLogger
	if err := getter.Get(config.LogSinkName, &logSink); err != nil {
		return nil, errors.Trace(err)
	}

	var serviceFactoryGetter servicefactory.ServiceFactoryGetter
	if err := getter.Get(config.ServiceFactoryName, &serviceFactoryGetter); err != nil {
		return nil, errors.Trace(err)
	}

	providerServiceFactoryGetter, err := config.GetProviderServiceFactoryGetter(getter, config.ProviderServiceFactoriesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, systemState, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineID := agent.CurrentConfig().Tag().Id()

	w, err := config.NewWorker(Config{
		Authority:    authority,
		Logger:       config.Logger,
		MachineID:    machineID,
		ModelWatcher: systemState,
		ModelMetrics: config.ModelMetrics,
		Controller: StatePoolController{
			StatePool: statePool,
		},
		LogSink:                      logSink,
		NewModelWorker:               config.NewModelWorker,
		ErrorDelay:                   jworker.RestartDelay,
		ServiceFactoryGetter:         serviceFactoryGetter,
		ProviderServiceFactoryGetter: providerServiceFactoryGetter,
		GetControllerConfig:          config.GetControllerConfig,
		StatePool:                    statePool,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// GetProviderServiceFactoryGetter returns a ProviderServiceFactoryGetter from
// the given dependency.Getter.
func GetProviderServiceFactoryGetter(getter dependency.Getter, name string) (ProviderServiceFactoryGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(factoryGetter servicefactory.ProviderServiceFactoryGetter) ProviderServiceFactoryGetter {
		return providerServiceFactoryGetter{factoryGetter: factoryGetter}
	})
}

// ControllerConfigService is an interface that returns the controller config.
type ControllerConfigService interface {
	ControllerConfig(ctx context.Context) (controller.Config, error)
}

// GetControllerConfig returns the controller config from the given service.
func GetControllerConfig(ctx context.Context, controllerConfigService ControllerConfigService) (controller.Config, error) {
	return controllerConfigService.ControllerConfig(ctx)
}

type providerServiceFactoryGetter struct {
	factoryGetter servicefactory.ProviderServiceFactoryGetter
}

// FactoryForModel returns a ProviderServiceFactory for the given model.
func (g providerServiceFactoryGetter) FactoryForModel(modelUUID string) ProviderServiceFactory {
	return providerServiceFactory{factory: g.factoryGetter.FactoryForModel(modelUUID)}
}

type providerServiceFactory struct {
	factory servicefactory.ProviderServiceFactory
}

func (f providerServiceFactory) Model() ProviderModelService {
	return f.factory.Model()
}

// Cloud returns the cloud service.
func (f providerServiceFactory) Cloud() ProviderCloudService {
	return f.factory.Cloud()
}

// Config returns the cloud service.
func (f providerServiceFactory) Config() ProviderConfigService {
	return f.factory.Config()
}

// Credential returns the credential service.
func (f providerServiceFactory) Credential() ProviderCredentialService {
	return f.factory.Credential()
}
