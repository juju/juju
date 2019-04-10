// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
)

//go:generate mockgen -package mocks -destination mocks/worker_mock.go gopkg.in/juju/worker.v1 Worker
//go:generate mockgen -package mocks -destination mocks/dependency_mock.go gopkg.in/juju/worker.v1/dependency Context
//go:generate mockgen -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs Environ,LXDProfiler
//go:generate mockgen -package mocks -destination mocks/base_mock.go github.com/juju/juju/api/base APICaller
//go:generate mockgen -package mocks -destination mocks/agent_mock.go github.com/juju/juju/agent Agent,Config

// ManifoldConfig describes the resources used by the instancemuter worker.
type ManifoldConfig struct {
	APICallerName string
	EnvironName   string
	AgentName     string

	Logger    Logger
	NewWorker func(Config) (worker.Worker, error)
	NewClient func(base.APICaller) InstanceMutaterAPI
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
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

func (config ManifoldConfig) newWorker(environ environs.Environ, apiCaller base.APICaller, agent agent.Agent) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// If we don't have a LXDProfiler, we should uninstall the worker as quickly
	// as possible.
	broker, ok := environ.(environs.LXDProfiler)
	if !ok {
		// If we don't have an LXDProfiler broker, there is no need to
		// run this worker.
		config.Logger.Debugf("uninstalling, not an LXD capable broker")
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

	w, err := config.NewWorker(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start machine instancemutater worker")
	}
	return w, nil
}

// Manifold returns a Manifold that encapsulates the instancemutater worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
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
type EnvironAPIStartFunc func(environs.Environ, base.APICaller, agent.Agent) (worker.Worker, error)

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
		Start: func(context dependency.Context) (worker.Worker, error) {
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			var environ environs.Environ
			if err := context.Get(config.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			return start(environ, apiCaller, agent)
		},
	}
}
