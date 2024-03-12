// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
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
	AgentName                  string
	AuthorityName              string
	StateName                  string
	ServiceFactoryName         string
	ProviderServiceFactoryName string
	LogSinkName                string

	NewWorker      func(Config) (worker.Worker, error)
	NewModelWorker NewModelWorkerFunc
	ModelMetrics   ModelMetrics
	Logger         Logger

	GetProviderServiceFactoryGetter GetProviderServiceFactoryGetterFunc
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
	if config.ProviderServiceFactoryName == "" {
		return errors.NotValidf("empty ProviderServiceFactoryName")
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
			config.ProviderServiceFactoryName,
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

	var controllerServiceFactory servicefactory.ControllerServiceFactory
	if err := getter.Get(config.ServiceFactoryName, &controllerServiceFactory); err != nil {
		return nil, errors.Trace(err)
	}

	providerServiceFactoryGetter, err := config.GetProviderServiceFactoryGetter(getter, config.ProviderServiceFactoryName)
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
		ControllerConfigGetter:       controllerServiceFactory.ControllerConfig(),
		NewModelWorker:               config.NewModelWorker,
		ErrorDelay:                   jworker.RestartDelay,
		ProviderServiceFactoryGetter: providerServiceFactoryGetter,
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

// Cloud returns the cloud service.
func (f providerServiceFactory) Cloud() ProviderCloudService {
	return f.factory.Cloud()
}

// Credential returns the credential service.
func (f providerServiceFactory) Credential() ProviderCredentialService {
	return f.factory.Credential()
}
