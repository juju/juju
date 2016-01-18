// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/version"
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
	// The machine agent uses these but the unit agent doesn't.
	if config.UpgradeStepsGateName != "" {
		inputs = append(inputs, config.UpgradeStepsGateName)
	}
	if config.UpgradeCheckGateName != "" {
		inputs = append(inputs, config.UpgradeCheckGateName)
	}

	return dependency.Manifold{
		Inputs: inputs,
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			// This wraps NewUpgrader for the convenience of the manifold. It should
			// eventually replace NewUpgrader.

			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}
			currentConfig := agent.CurrentConfig()

			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			upgraderFacade := upgrader.NewState(apiCaller)

			var upgradeStepsWaiter gate.Waiter
			if config.UpgradeStepsGateName == "" {
				upgradeStepsWaiter = gate.NewLock()
			} else {
				if config.PreviousAgentVersion == version.Zero {
					return nil, errors.New("previous agent version not specified")
				}
				if err := getResource(config.UpgradeStepsGateName, &upgradeStepsWaiter); err != nil {
					return nil, err
				}
			}

			var initialCheckUnlocker gate.Unlocker
			if config.UpgradeCheckGateName == "" {
				initialCheckUnlocker = gate.NewLock()
			} else {
				if err := getResource(config.UpgradeCheckGateName, &initialCheckUnlocker); err != nil {
					return nil, err
				}
			}

			return NewAgentUpgrader(
				upgraderFacade,
				currentConfig,
				config.PreviousAgentVersion,
				upgradeStepsWaiter,
				initialCheckUnlocker,
			)
		},
	}
}
