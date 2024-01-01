// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/worker/gate"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	OpenStateForUpgrade  func() (*state.StatePool, error)
	PreUpgradeSteps      upgrades.PreUpgradeStepsFunc
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
			var localAgent agent.Agent
			if err := context.Get(config.AgentName, &localAgent); err != nil {
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

			// Get upgradeSteps completed lock.
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
			// Application tag for CAAS operator; controller,
			// machine or unit tag for agents.
			agentTag := localAgent.CurrentConfig().Tag()
			isOperator := agentTag.Kind() == names.ApplicationTagKind

			var isController bool
			if !isOperator {
				isController, err = apiagent.IsController(apiConn, agentTag)
				if err != nil {
					return nil, errors.Trace(err)
				}
			}
			return NewWorker(
				upgradeStepsLock,
				localAgent,
				apiConn,
				isController,
				config.OpenStateForUpgrade,
				config.PreUpgradeSteps,
				retry.CallArgs{
					Clock:    clock.WallClock,
					Delay:    2 * time.Minute,
					Attempts: 5,
				},
				statusSetter,
				isOperator,
			)
		},
	}
}
