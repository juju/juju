// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	NewDeployContext func(st *apideployer.State, agentConfig agent.Config) Context
}

// Manifold returns a dependency manifold that runs a deployer worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {

	// newWorker trivially wraps NewDeployer for use in a util.PostUpgradeManifold.
	//
	// It's not tested at the moment, because the scaffolding
	// necessary is too unwieldy/distracting to introduce at this point.
	newWorker := func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		cfg := a.CurrentConfig()
		// Grab the tag and ensure that it's for a machine.
		tag, ok := cfg.Tag().(names.MachineTag)
		if !ok {
			return nil, errors.New("agent's tag is not a machine tag")
		}

		// Get API connection.
		apiConn, ok := apiCaller.(api.Connection)
		if !ok {
			return nil, errors.New("unable to obtain api.Connection")
		}

		// Get the machine agent's jobs.
		entity, err := apiConn.Agent().Entity(tag)
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

		apiDeployer := apiConn.Deployer()
		context := config.NewDeployContext(apiDeployer, cfg)
		w, err := NewDeployer(apiDeployer, context)
		if err != nil {
			return nil, errors.Annotate(err, "cannot start unit agent deployer worker")
		}
		return w, nil
	}

	return util.PostUpgradeManifold(config.PostUpgradeManifoldConfig, newWorker)
}
