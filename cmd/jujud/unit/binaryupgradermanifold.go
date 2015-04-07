// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/upgrader"
)

// BinaryUpgraderManifoldConfig defines the names of the manifolds on which a
// BinaryUpgraderManifold will depend.
type BinaryUpgraderManifoldConfig struct {
	AgentName         string
	ApiConnectionName string
}

// BinaryUpgraderManifold returns a dependency manifold that runs an upgrader
// worker, using the resource names defined in the supplied config.
//
// It should really be defined in worker/upgrader instead, but import loops render
// this impractical for the time being.
func BinaryUpgraderManifold(config BinaryUpgraderManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiConnectionName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {

			// Get the dependencies.
			var agent agent.Agent
			if !getResource(config.AgentName, &agent) {
				return nil, dependency.ErrUnmetDependencies
			}
			var apiConnection *api.State
			if !getResource(config.ApiConnectionName, &apiConnection) {
				return nil, dependency.ErrUnmetDependencies
			}
			currentConfig := agent.CurrentConfig()
			upgraderFacade := apiConnection.Upgrader()

			// TODO(fwereade): this should be in Upgrader itself, but it's
			// inconvenient to do that and leave the machine agent double-
			// calling it. When the machine agent uses a manifold to run its
			// upgrader we can move this call.
			err := upgraderFacade.SetVersion(currentConfig.Tag().String(), version.Current)
			if err != nil {
				return nil, errors.Annotate(err, "cannot set unit agent version")
			}

			// Start the upgrader.
			return upgrader.NewUpgrader(
				upgraderFacade,
				currentConfig,
				currentConfig.UpgradedToVersion(),
				func() bool { return false },
			), nil
		},
	}
}
