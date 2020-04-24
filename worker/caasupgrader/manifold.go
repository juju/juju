// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgrader

import (
	"github.com/juju/errors"
	"github.com/juju/version"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/caasoperatorupgrader"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/worker/gate"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	UpgradeCheckGateName string
	PreviousAgentVersion version.Number
}

// Manifold returns a dependency manifold that runs an upgrader
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{
		config.AgentName,
		config.APICallerName,
	}

	// The machine agent uses these but the application agent doesn't.
	if config.UpgradeStepsGateName != "" {
		inputs = append(inputs, config.UpgradeStepsGateName)
	}
	if config.UpgradeCheckGateName != "" {
		inputs = append(inputs, config.UpgradeCheckGateName)
	}

	return dependency.Manifold{
		Inputs: inputs,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.PreviousAgentVersion == version.Zero {
				return nil, errors.New("previous agent version not specified")
			}

			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			currentConfig := agent.CurrentConfig()

			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			upgraderFacade := upgrader.NewState(apiCaller)
			operatorUpgraderFacade := caasoperatorupgrader.NewClient(apiCaller)

			var upgradeStepsWaiter gate.Waiter
			if config.UpgradeStepsGateName == "" {
				upgradeStepsWaiter = gate.NewLock()
			} else {
				if config.PreviousAgentVersion == version.Zero {
					return nil, errors.New("previous agent version not specified")
				}
				if err := context.Get(config.UpgradeStepsGateName, &upgradeStepsWaiter); err != nil {
					return nil, err
				}
			}

			var initialCheckUnlocker gate.Unlocker
			if config.UpgradeCheckGateName == "" {
				initialCheckUnlocker = gate.NewLock()
			} else {
				if err := context.Get(config.UpgradeCheckGateName, &initialCheckUnlocker); err != nil {
					return nil, err
				}
			}

			return NewUpgrader(Config{
				UpgraderClient:              upgraderFacade,
				CAASOperatorUpgrader:        operatorUpgraderFacade,
				AgentTag:                    currentConfig.Tag(),
				OrigAgentVersion:            config.PreviousAgentVersion,
				UpgradeStepsWaiter:          upgradeStepsWaiter,
				InitialUpgradeCheckComplete: initialCheckUnlocker,
			})
		},
	}
}
