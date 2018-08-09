// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// ManifoldConfig holds the information necessary for the dependency engine to
// to run an upgrade-series worker.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string

	Logger    Logger
	NewFacade func(base.APICaller, names.Tag) Facade
	NewWorker func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker function")
	}
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade function")
	}
	return nil
}

// Manifold returns a dependency manifold that runs an upgrade-series worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return engine.AgentAPIManifold(typedConfig, config.newWorker)
}

// newWorker wraps NewWorker for use in a engine.AgentAPIManifold.
func (config ManifoldConfig) newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Ensure that we have a machine tag.
	agentCfg := a.CurrentConfig()
	tag, ok := agentCfg.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("expected a machine tag, got %v", tag)
	}

	// We curry the NewFacade method and pass it as a factory.
	// This is so the worker can use the API server in different contexts.
	newFacade := func(t names.Tag) Facade {
		return config.NewFacade(apiCaller, t)
	}

	cfg := Config{
		Tag:           tag,
		Logger:        config.Logger,
		FacadeFactory: newFacade,
		Service:       &serviceAccess{},
	}

	w, err := config.NewWorker(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "starting machine upgrade series worker")
	}
	return w, nil
}
