// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseriesworker

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
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
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a deployer worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return engine.AgentAPIManifold(typedConfig, config.newWorker)
}

// newWorker trivially wraps NewWorker for use in a engine.AgentAPIManifold.
func (config ManifoldConfig) newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Grab the tag and ensure that it's for a machine.
	agentCfg := a.CurrentConfig()
	tag, ok := agentCfg.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.New("agent's tag is not a machine tag")
	}

	cfg := Config{
		Tag:    tag,
		Logger: config.Logger,
		Facade: config.NewFacade(apiCaller, tag),
	}

	w, err := config.NewWorker(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start machine upgrade series worker")
	}
	return w, nil
}
