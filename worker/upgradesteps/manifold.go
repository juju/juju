// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/gate"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	OpenStateForUpgrade  func() (*state.StatePool, error)
	PreUpgradeSteps      func(*state.StatePool, agent.Config, bool, bool, bool) error
	NewAgentStatusSetter func(apiConn api.Connection) (StatusSetter, error)
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
		Start: func(context dependency.Context) (worker.Worker, error) {
			// Sanity checks
			if config.OpenStateForUpgrade == nil {
				return nil, errors.New("missing OpenStateForUpgrade in config")
			}
			if config.PreUpgradeSteps == nil {
				return nil, errors.New("missing PreUpgradeSteps in config")
			}

			// Get the agent.
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}

			// Get API connection.
			// TODO(fwereade): can we make the worker use an
			// APICaller instead? should be able to depend on
			// the Engine to abort us when conn is closed...
			var apiConn api.Connection
			if err := context.Get(config.APICallerName, &apiConn); err != nil {
				return nil, errors.Trace(err)
			}

			// Get upgradesteps completed lock.
			var upgradeStepsLock gate.Lock
			if err := context.Get(config.UpgradeStepsGateName, &upgradeStepsLock); err != nil {
				return nil, errors.Trace(err)
			}

			// Get a component capable of setting machine status
			// to indicate progress to the user.
			statusSetter, err := config.NewAgentStatusSetter(apiConn)
			if err != nil {
				return nil, errors.Trace(err)
			}
			// application tag for CAAS operator; controller, machine or unit tag for agents.
			isOperator := agent.CurrentConfig().Tag().Kind() == names.ApplicationTagKind
			if isOperator {
				return NewWorker(
					upgradeStepsLock,
					agent,
					apiConn,
					false,
					config.OpenStateForUpgrade,
					config.PreUpgradeSteps,
					statusSetter,
					isOperator,
				)
			}

			isController, err := apiagent.IsController(apiConn, agent.CurrentConfig().Tag())
			if err != nil {
				return nil, errors.Trace(err)
			}
			return NewWorker(
				upgradeStepsLock,
				agent,
				apiConn,
				isController,
				config.OpenStateForUpgrade,
				config.PreUpgradeSteps,
				statusSetter,
				isOperator,
			)
		},
	}
}
