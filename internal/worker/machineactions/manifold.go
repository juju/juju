// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/machinelock"
)

// ManifoldConfig describes the dependencies of a machine action runner.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	MachineLock   machinelock.Lock

	NewFacade func(base.APICaller) Facade
	NewWorker func(WorkerConfig) (worker.Worker, error)
}

// start is used by engine.AgentAPIManifold to create a StartFunc.
func (config ManifoldConfig) start(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	machineTag, ok := a.CurrentConfig().Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("this manifold can only be used inside a machine")
	}
	machineActionsFacade := config.NewFacade(apiCaller)
	return config.NewWorker(WorkerConfig{
		Facade:       machineActionsFacade,
		MachineTag:   machineTag,
		MachineLock:  config.MachineLock,
		HandleAction: HandleAction,
	})
}

// Manifold returns a dependency.Manifold as configured.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return engine.AgentAPIManifold(typedConfig, config.start)
}
