// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/service"
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

	// Partially apply the upgrader factory function so we only need to request
	// using the getter for the to/from OS series.
	newUpgrader := func() (Upgrader, error) {
		return NewUpgrader(service.NewServiceManagerWithDefaults(), config.Logger)
	}

	cfg := Config{
		Logger:          config.Logger,
		Facade:          config.NewFacade(apiCaller, tag),
		UnitDiscovery:   &datadirAgents{agentCfg.DataDir()},
		UpgraderFactory: newUpgrader,
	}

	w, err := config.NewWorker(cfg)
	return w, errors.Annotate(err, "starting machine upgrade series worker")
}

type datadirAgents struct {
	datadir string
}

// Units returns the unit tags of the deployed units on the machine.
// This method uses the service.FindAgents method by looking for matching
// names in the data directory. Calling an API method would be an alternative
// except the current upgrader facade doesn't support the Units method.
func (d *datadirAgents) Units() ([]names.UnitTag, error) {
	_, units, _, err := service.FindAgents(d.datadir)
	if err != nil {
		return nil, errors.Annotate(err, "finding deployed units")
	}
	var result []names.UnitTag
	for _, name := range units {
		// We know that this string parses correctly and is a unit tag
		// from the FindAgents function.
		tag, _ := names.ParseTag(name)
		result = append(result, tag.(names.UnitTag))
	}
	return result, nil
}
