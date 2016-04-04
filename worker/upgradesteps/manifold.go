// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	apimachiner "github.com/juju/juju/api/machiner"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	OpenStateForUpgrade  func() (*state.State, error)
	PreUpgradeSteps      func(*state.State, agent.Config, bool, bool) error
}

// Manifold returns a dependency manifold that runs an upgrader
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.UpgradeStepsGateName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			// Sanity checks
			if config.OpenStateForUpgrade == nil {
				return nil, errors.New("missing OpenStateForUpgrade in config")
			}
			if config.PreUpgradeSteps == nil {
				return nil, errors.New("missing PreUpgradeSteps in config")
			}

			// Get machine agent.
			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}

			// Grab the tag and ensure that it's for a machine.
			tag, ok := agent.CurrentConfig().Tag().(names.MachineTag)
			if !ok {
				return nil, errors.New("agent's tag is not a machine tag")
			}

			// Get API connection.
			// TODO(fwereade): can we make the worker use an
			// APICaller instead? should be able to depend on
			// the Engine to abort us when conn is closed...
			var apiConn api.Connection
			if err := getResource(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}

			// Get the machine agent's jobs.
			// TODO(fwereade): use appropriate facade!
			agentFacade := apiagent.NewState(apiConn)
			entity, err := agentFacade.Entity(tag)
			if err != nil {
				return nil, err
			}
			jobs := entity.Jobs()

			// Get machine instance for setting status on.
			// TODO(fwereade): use appropriate facade!
			machinerFacade := apimachiner.NewState(apiConn)
			machine, err := machinerFacade.Machine(tag)
			if err != nil {
				return nil, err
			}

			// Get upgradesteps completed lock.
			var upgradeStepsLock gate.Lock
			if err := getResource(config.UpgradeStepsGateName, &upgradeStepsLock); err != nil {
				return nil, err
			}

			return NewWorker(
				upgradeStepsLock,
				agent,
				apiConn,
				jobs,
				config.OpenStateForUpgrade,
				config.PreUpgradeSteps,
				machine,
			)
		},
	}
}
