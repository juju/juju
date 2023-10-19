// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/worker/gate"
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Errorf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
	Debugf(string, ...any)
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	OpenStateForUpgrade  func() (*state.StatePool, SystemState, error)
	PreUpgradeSteps      upgrades.PreUpgradeStepsFunc
	NewAgentStatusSetter func(base.APICaller) (StatusSetter, error)
	Logger               Logger
}

// Validate checks that the config is valid.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if c.UpgradeStepsGateName == "" {
		return errors.NotValidf("empty UpgradeStepsGateName")
	}
	if c.OpenStateForUpgrade == nil {
		return errors.NotValidf("nil OpenStateForUpgrade")
	}
	if c.PreUpgradeSteps == nil {
		return errors.NotValidf("nil PreUpgradeSteps")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
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
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the agent.
			var localAgent agent.Agent
			if err := context.Get(config.AgentName, &localAgent); err != nil {
				return nil, errors.Trace(err)
			}

			// Get API connection.
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			// Get upgradeSteps completed lock.
			var upgradeStepsLock gate.Lock
			if err := context.Get(config.UpgradeStepsGateName, &upgradeStepsLock); err != nil {
				return nil, errors.Trace(err)
			}

			// Get a component capable of setting machine status
			// to indicate progress to the user.
			statusSetter, err := config.NewAgentStatusSetter(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			// Application tag for CAAS operator; controller,
			// machine or unit tag for agents.
			agentTag := localAgent.CurrentConfig().Tag()
			isController, err := apiagent.IsController(apiCaller, agentTag)
			if err != nil {
				return nil, errors.Trace(err)
			}

			return NewWorker(
				upgradeStepsLock,
				localAgent,
				apiCaller,
				isController,
				config.OpenStateForUpgrade,
				config.PreUpgradeSteps,
				statusSetter,
				config.Logger,
			)
		},
	}
}
