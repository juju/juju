// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/worker/gate"
)

type (
	PreUpgradeStepsFunc = upgrades.PreUpgradeStepsFunc
	UpgradeStepsFunc    = upgrades.UpgradeStepsFunc
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Errorf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
	Debugf(string, ...any)
}

// StatusSetter defines the single method required to set an agent's
// status.
type StatusSetter interface {
	SetStatus(setableStatus status.Status, info string, data map[string]any) error
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	ServiceFactoryName   string
	PreUpgradeSteps      upgrades.PreUpgradeStepsFunc
	UpgradeSteps         upgrades.UpgradeStepsFunc
	NewAgentStatusSetter func(base.APICaller) (StatusSetter, error)
	Logger               Logger
	Clock                clock.Clock
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
	if c.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if c.PreUpgradeSteps == nil {
		return errors.NotValidf("nil PreUpgradeSteps")
	}
	if c.UpgradeSteps == nil {
		return errors.NotValidf("nil UpgradeSteps")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
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
			config.ServiceFactoryName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the agent.
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
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

			agentTag := agent.CurrentConfig().Tag()
			isController, err := apiagent.IsController(apiCaller, agentTag)
			if err != nil {
				return nil, errors.Trace(err)
			}

			if !isController {
				// Create a new machine worker. As this is purely a
				// machine worker, we don't need to worry about the
				// upgrade service.
				return NewMachineWorker(
					upgradeStepsLock,
					agent,
					apiCaller,
					config.PreUpgradeSteps,
					config.UpgradeSteps,
					config.Logger,
				), nil
			}

			// Get a component capable of setting machine status
			// to indicate progress to the user.
			statusSetter, err := config.NewAgentStatusSetter(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}

			// Service factory is used to get the upgrade service and
			// then we can locate all the model uuids.
			var serviceFactoryGetter servicefactory.ControllerServiceFactory
			if err := context.Get(config.ServiceFactoryName, &serviceFactoryGetter); err != nil {
				return nil, errors.Trace(err)
			}

			return NewControllerWorker(
				upgradeStepsLock,
				agent,
				apiCaller,
				serviceFactoryGetter.Upgrade(),
				config.PreUpgradeSteps,
				config.UpgradeSteps,
				statusSetter,
				config.Logger,
				config.Clock,
			)
		},
	}
}
