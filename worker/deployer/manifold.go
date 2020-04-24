// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/cmd/jujud/agent/engine"
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
	typedConfig := engine.AgentAPIManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return engine.AgentAPIManifold(typedConfig, config.newWorker)
}

// newWorker trivially wraps NewDeployer for use in a engine.AgentAPIManifold.
//
// It's not tested at the moment, because the scaffolding
// necessary is too unwieldy/distracting to introduce at this point.
func (config ManifoldConfig) newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	cfg := a.CurrentConfig()
	// Grab the tag and ensure that it's for a machine.
	if cfg.Tag().Kind() != names.MachineTagKind {
		return nil, errors.New("agent's tag is not a machine tag")
	}
	deployerFacade := apideployer.NewState(apiCaller)
	context := config.NewDeployContext(deployerFacade, cfg)
	w, err := NewDeployer(deployerFacade, context)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start unit agent deployer worker")
	}
	return w, nil
}
