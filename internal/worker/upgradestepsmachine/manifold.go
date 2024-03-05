// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepsmachine

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	"github.com/juju/juju/internal/worker/gate"
)

// StatusSetter defines the single method required to set an agent's
// status.
type StatusSetter interface {
	SetStatus(setableStatus status.Status, info string, data map[string]any) error
}

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
	PreUpgradeSteps      upgrades.PreUpgradeStepsFunc
	UpgradeSteps         upgrades.UpgradeStepsFunc
	NewAgentStatusSetter func(context.Context, base.APICaller) (upgradesteps.StatusSetter, error)
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
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the agent.
			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}

			// Get API connection.
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			// Get upgradeSteps completed lock.
			var upgradeStepsLock gate.Lock
			if err := getter.Get(config.UpgradeStepsGateName, &upgradeStepsLock); err != nil {
				return nil, errors.Trace(err)
			}

			// Get a component capable of setting machine status
			// to indicate progress to the user.
			statusSetter, err := config.NewAgentStatusSetter(ctx, apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}

			// Create a new machine worker. As this is purely a
			// machine worker, we don't need to worry about the
			// upgrade service.
			return NewMachineWorker(
				upgradeStepsLock,
				agent,
				apiCaller,
				config.PreUpgradeSteps,
				config.UpgradeSteps,
				statusSetter,
				config.Logger,
				config.Clock,
			), nil

		},
	}
}
