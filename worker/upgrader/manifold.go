// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/upgrader"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/worker/gate"
)

// Clock represents the clock methods this worker uses.
type Clock interface {
	After(time.Duration) <-chan time.Time
}

// Logger represents the logging methods used by this package.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	UpgradeCheckGateName string
	PreviousAgentVersion version.Number
	Logger               Logger
	Clock                Clock
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
		Start: func(context dependency.Context) (worker.Worker, error) {

			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			currentConfig := agent.CurrentConfig()

			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			upgraderFacade := upgrader.NewClient(apiCaller)

			// If there is no UpgradeStepsGateName, the worker should
			// report the running version and exit.
			var upgradeStepsWaiter gate.Waiter
			if config.UpgradeStepsGateName != "" {
				if config.PreviousAgentVersion == version.Zero {
					return nil, errors.New("previous agent version not specified")
				}
				if err := context.Get(config.UpgradeStepsGateName, &upgradeStepsWaiter); err != nil {
					return nil, err
				}
			}

			var initialCheckUnlocker gate.Unlocker
			if config.UpgradeCheckGateName != "" {
				if err := context.Get(config.UpgradeCheckGateName, &initialCheckUnlocker); err != nil {
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
