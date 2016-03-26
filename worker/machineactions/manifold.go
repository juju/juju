// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
	"github.com/juju/names"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentApiManifoldConfig util.AgentApiManifoldConfig
	NewFacade              func(base.APICaller) Facade
	NewWorker              func(WorkerConfig) (worker.Worker, error)
}

// Manifold returns a dependency manifold that runs a hook retry strategy worker,
// using the agent name and the api connection resources named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.AgentApiManifold(util.AgentApiManifoldConfig(config.AgentApiManifoldConfig), config.start)
	return manifold
}

func (mc ManifoldConfig) start(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	machineTag, ok := a.CurrentConfig().Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("this manifold can only be used inside a machine")
	}
	machineActionsFacade := mc.NewFacade(apiCaller)
	return mc.NewWorker(WorkerConfig{
		Facade:       machineActionsFacade,
		MachineTag:   machineTag,
		HandleAction: HandleAction,
	})
}
