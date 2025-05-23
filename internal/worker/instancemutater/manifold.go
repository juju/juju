// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
)

// ModelManifoldConfig describes the resources used by the instancemuter worker.
type ModelManifoldConfig struct {
	APICallerName string
	EnvironName   string
	AgentName     string

	Logger    logger.Logger
	NewWorker func(context.Context, Config) (worker.Worker, error)
	NewClient func(base.APICaller) InstanceMutaterAPI
}

// Validate validates the manifold configuration.
func (config ModelManifoldConfig) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.EnvironName == "" {
		return errors.NotValidf("empty EnvironName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	return nil
}

func (config ModelManifoldConfig) newWorker(ctx context.Context, environ environs.Environ, apiCaller base.APICaller, agent agent.Agent) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	// If we don't have a LXDProfiler, we should uninstall the worker as quickly
	// as possible.
	broker, ok := environ.(environs.LXDProfiler)
	if !ok {
		// If we don't have an LXDProfiler broker, there is no need to
		// run this worker.
		config.Logger.Debugf(ctx, "Uninstalling worker because the broker is not a LXDProfiler %T", environ)
		return nil, dependency.ErrUninstall
	}
	facade := config.NewClient(apiCaller)
	agentConfig := agent.CurrentConfig()
	cfg := Config{
		Logger:      config.Logger,
		Facade:      facade,
		Broker:      broker,
		AgentConfig: agentConfig,
		Tag:         agentConfig.Tag(),
	}

	w, err := config.NewWorker(ctx, cfg)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start model instance-mutater worker")
	}
	return w, nil
}

// ModelManifold returns a Manifold that encapsulates the instancemutater worker.
func ModelManifold(config ModelManifoldConfig) dependency.Manifold {
	typedConfig := EnvironAPIConfig{
		EnvironName:   config.EnvironName,
		APICallerName: config.APICallerName,
		AgentName:     config.AgentName,
	}
	return EnvironAPIManifold(typedConfig, config.newWorker)
}

// EnvironAPIConfig represents a typed manifold starter func, that handles
// getting resources from the configuration.
type EnvironAPIConfig struct {
	EnvironName   string
	APICallerName string
	AgentName     string
}

// EnvironAPIStartFunc encapsulates creation of a worker based on the environ
// and APICaller.
type EnvironAPIStartFunc func(context.Context, environs.Environ, base.APICaller, agent.Agent) (worker.Worker, error)

// EnvironAPIManifold returns a dependency.Manifold that calls the supplied
// start func with the API and envrion resources defined in the config
// (once those resources are present).
func EnvironAPIManifold(config EnvironAPIConfig, start EnvironAPIStartFunc) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.EnvironName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			var environ environs.Environ
			if err := getter.Get(config.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			return start(ctx, environ, apiCaller, agent)
		},
	}
}

// MachineManifoldConfig describes the resources used by the instancemuter worker.
type MachineManifoldConfig struct {
	APICallerName string
	BrokerName    string
	AgentName     string

	Logger    logger.Logger
	NewWorker func(context.Context, Config) (worker.Worker, error)
	NewClient func(base.APICaller) InstanceMutaterAPI
}

// Validate validates the manifold configuration.
func (config MachineManifoldConfig) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	return nil
}

func (config MachineManifoldConfig) newWorker(ctx context.Context, instanceBroker environs.InstanceBroker, apiCaller base.APICaller, agent agent.Agent) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	// If we don't have a LXDProfiler, we should uninstall the worker as quickly
	// as possible.
	broker, ok := instanceBroker.(environs.LXDProfiler)
	if !ok {
		// If we don't have an LXDProfiler broker, there is no need to
		// run this worker.
		config.Logger.Debugf(ctx, "Uninstalling worker because the broker is not a LXDProfiler %T", instanceBroker)
		return nil, dependency.ErrUninstall
	}
	agentConfig := agent.CurrentConfig()
	tag := agentConfig.Tag()
	if _, ok := tag.(names.MachineTag); !ok {
		config.Logger.Warningf(ctx, "cannot start a ContainerWorker on a %q, not starting", tag.Kind())
		return nil, dependency.ErrUninstall
	}
	facade := config.NewClient(apiCaller)
	cfg := Config{
		Logger:      config.Logger,
		Facade:      facade,
		Broker:      broker,
		AgentConfig: agentConfig,
		Tag:         tag,
	}

	w, err := config.NewWorker(ctx, cfg)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start machine instancemutater worker")
	}
	return w, nil
}

// MachineManifold returns a Manifold that encapsulates the instancemutater worker.
func MachineManifold(config MachineManifoldConfig) dependency.Manifold {
	typedConfig := BrokerAPIConfig{
		BrokerName:    config.BrokerName,
		APICallerName: config.APICallerName,
		AgentName:     config.AgentName,
	}
	return BrokerAPIManifold(typedConfig, config.newWorker)
}

// BrokerAPIConfig represents a typed manifold starter func, that handles
// getting resources from the configuration.
type BrokerAPIConfig struct {
	BrokerName    string
	APICallerName string
	AgentName     string
}

// BrokerAPIStartFunc encapsulates creation of a worker based on the environ
// and APICaller.
type BrokerAPIStartFunc func(context.Context, environs.InstanceBroker, base.APICaller, agent.Agent) (worker.Worker, error)

// BrokerAPIManifold returns a dependency.Manifold that calls the supplied
// start func with the API and envrion resources defined in the config
// (once those resources are present).
func BrokerAPIManifold(config BrokerAPIConfig, start BrokerAPIStartFunc) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.BrokerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			var broker environs.InstanceBroker
			if err := getter.Get(config.BrokerName, &broker); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			return start(ctx, broker, apiCaller, agent)
		},
	}
}
