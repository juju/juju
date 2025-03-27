// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/upgrader"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/worker/gate"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	UpgradeCheckGateName string
	PreviousAgentVersion semversion.Number
	Logger               logger.Logger
	Clock                clock.Clock
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
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {

			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			currentConfig := agent.CurrentConfig()

			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			upgraderFacade := upgrader.NewClient(apiCaller)

			// If there is no UpgradeStepsGateName, the worker should
			// report the running version and exit.
			var upgradeStepsWaiter gate.Waiter
			if config.UpgradeStepsGateName != "" {
				if config.PreviousAgentVersion == semversion.Zero {
					return nil, errors.New("previous agent version not specified")
				}
				if err := getter.Get(config.UpgradeStepsGateName, &upgradeStepsWaiter); err != nil {
					return nil, err
				}
			}

			var initialCheckUnlocker gate.Unlocker
			if config.UpgradeCheckGateName != "" {
				if err := getter.Get(config.UpgradeCheckGateName, &initialCheckUnlocker); err != nil {
					return nil, err
				}
			}

			return NewAgentUpgrader(Config{
				Clock:                       config.Clock,
				Logger:                      config.Logger,
				Client:                      upgraderFacade,
				AgentConfig:                 currentConfig,
				OrigAgentVersion:            config.PreviousAgentVersion,
				UpgradeStepsWaiter:          upgradeStepsWaiter,
				InitialUpgradeCheckComplete: initialCheckUnlocker,
				CheckDiskSpace:              upgrades.CheckFreeDiskSpace,
			})
		},
	}
}
