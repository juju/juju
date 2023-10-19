// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	domainupgrade "github.com/juju/juju/domain/upgrade"
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

			var upgradeService UpgradeService
			if isController {
				// Service factory is used to get the upgrade service and
				// then we can locate all the model uuids.
				var serviceFactoryGetter servicefactory.ControllerServiceFactory
				if err := context.Get(config.ServiceFactoryName, &serviceFactoryGetter); err != nil {
					return nil, errors.Trace(err)
				}
				upgradeService = serviceFactoryGetter.Upgrade()
			} else {
				// Set a default upgrade service that does nothing. This is
				// to prevent any potential panics.
				upgradeService = noopUpgradeService{}
			}

			return NewWorker(
				upgradeStepsLock,
				localAgent,
				apiCaller,
				upgradeService,
				isController,
				agentTag,
				config.PreUpgradeSteps,
				config.UpgradeSteps,
				statusSetter,
				config.Logger,
			)
		},
	}
}

type noopUpgradeService struct{}

// SetControllerDone marks the supplied controllerID as having
// completed its upgrades. When SetControllerDone is called by the
// last provisioned controller, the upgrade will be archived.
func (noopUpgradeService) SetControllerDone(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error {
	return errors.NotSupportedf("not running on controller, set controller done")
}

// ActiveUpgrade returns the uuid of the current active upgrade.
// If there are no active upgrades, return a NotFound error
func (noopUpgradeService) ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error) {
	return domainupgrade.UUID(""), errors.NotSupportedf("not running on controller, active upgrade")
}

// UpgradeInfo returns the upgrade info for the given upgrade.
func (noopUpgradeService) UpgradeInfo(ctx context.Context, upgradeUUID domainupgrade.UUID) (upgrade.Info, error) {
	return upgrade.Info{}, errors.NotSupportedf("not running on controller, upgrade info")
}

// WatchForUpgradeState creates a watcher which notifies when the upgrade
// has reached the given state.
func (noopUpgradeService) WatchForUpgradeState(ctx context.Context, upgradeUUID domainupgrade.UUID, state upgrade.State) (watcher.NotifyWatcher, error) {
	return nil, errors.NotSupportedf("not running on controller, watch for upgrade state")
}
