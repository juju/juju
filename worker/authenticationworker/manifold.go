// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/keyupdater"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	MachineID          string
	BootstrapMachineID string
}

// Manifold returns a dependency manifold that runs a authenticationworker worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {

	newWorker := func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		// If not a local provider bootstrap machine, start the worker to
		// manage SSH keys.
		agentConfig := a.CurrentConfig()
		providerType := agentConfig.Value(agent.ProviderType)
		if providerType == provider.Local && config.MachineID == config.BootstrapMachineID {
			return nil, nil
		}
		apiConn, ok := apiCaller.(api.Connection)
		if !ok {
			return nil, errors.New("unable to obtain api.Connection")
		}

		w, err := NewWorker(keyupdater.NewState(apiConn), agentConfig)
		if err != nil {
			return nil, errors.Annotate(err, "cannot start ssh auth-keys updater worker")
		}
		return w, nil

		return nil, nil
	}

	return util.PostUpgradeManifold(config.PostUpgradeManifoldConfig, newWorker)
}
