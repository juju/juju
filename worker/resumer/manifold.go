// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiresumer "github.com/juju/juju/api/resumer"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.PostUpgradeManifoldConfig

// Manifold returns a dependency manifold that runs a resumer worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.PostUpgradeManifold(util.PostUpgradeManifoldConfig(config), newWorker)
}

func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	cfg := a.CurrentConfig()
	// Grab the tag and ensure that it's for a machine.
	tag, ok := cfg.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.New("this manifold may only be used inside a machine agent")
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

	var isModelManager bool
	for _, job := range entity.Jobs() {
		if job == multiwatcher.JobManageModel {
			isModelManager = true
			break
		}
	}
	if !isModelManager {
		return nil, dependency.ErrMissing
	}

	return NewResumer(apiresumer.NewAPI(apiCaller)), nil
}
