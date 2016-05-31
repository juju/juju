// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/cmd/jujud/agent/util"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName        string
	APICallerName    string
	NewDeployContext func(st *apideployer.State, agentConfig agent.Config) Context
}

// Manifold returns a dependency manifold that runs a deployer worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := util.AgentApiManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return util.AgentApiManifold(typedConfig, config.newWorker)
}

// newWorker trivially wraps NewDeployer for use in a util.AgentApiManifold.
//
// It's not tested at the moment, because the scaffolding
// necessary is too unwieldy/distracting to introduce at this point.
func (config ManifoldConfig) newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	cfg := a.CurrentConfig()
	// Grab the tag and ensure that it's for a machine.
	tag, ok := cfg.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.New("agent's tag is not a machine tag")
	}

	// Get the machine agent's jobs.
	// TODO(fwereade): this functionality should be on the
	// deployer facade instead.
	agentFacade := apiagent.NewState(apiCaller)
	entity, err := agentFacade.Entity(tag)
	if err != nil {
		return nil, err
	}

	var isUnitHoster bool
	for _, job := range entity.Jobs() {
		if job == multiwatcher.JobHostUnits {
			isUnitHoster = true
			break
		}
	}

	if !isUnitHoster {
		return nil, dependency.ErrUninstall
	}

	deployerFacade := apideployer.NewState(apiCaller)
	context := config.NewDeployContext(deployerFacade, cfg)
	w, err := NewDeployer(deployerFacade, context)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start unit agent deployer worker")
	}
	return w, nil
}
