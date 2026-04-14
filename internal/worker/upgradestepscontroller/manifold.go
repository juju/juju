// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradestepscontroller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	"github.com/juju/juju/internal/worker/gate"
)

// ControllerWorkerFunc defines a function that returns a worker.Worker
// which runs the upgrade steps for a controller.
type ControllerWorkerFunc func(
	gate.Lock,
	agent.Agent, base.APICaller,
	UpgradeService,
	upgrades.PreUpgradeStepsFunc,
	upgrades.UpgradeStepsFunc,
	upgradesteps.StatusSetter,
	logger.Logger,
	clock.Clock,
) (worker.Worker, error)

// NewAgentStatusSetterFunc is a function that creates a new StatusSetter
// for the agent.
type NewAgentStatusSetterFunc func(context.Context, base.APICaller) (upgradesteps.StatusSetter, error)

// GetUpgradeServiceFunc is a function that retrieves the UpgradeService from a
// dependency.Getter.
type GetUpgradeServiceFunc func(dependency.Getter, string) (UpgradeService, error)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string
	DomainServicesName   string
	PreUpgradeSteps      upgrades.PreUpgradeStepsFunc
	UpgradeSteps         upgrades.UpgradeStepsFunc
	NewAgentStatusSetter NewAgentStatusSetterFunc
	NewControllerWorker  ControllerWorkerFunc
	GetUpgradeService    GetUpgradeServiceFunc
	Logger               logger.Logger
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
	if c.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if c.PreUpgradeSteps == nil {
		return errors.NotValidf("nil PreUpgradeSteps")
	}
	if c.UpgradeSteps == nil {
		return errors.NotValidf("nil UpgradeSteps")
	}
	if c.NewAgentStatusSetter == nil {
		return errors.NotValidf("nil NewAgentStatusSetter")
	}
	if c.NewControllerWorker == nil {
		return errors.NotValidf("nil NewControllerWorker")
	}
	if c.GetUpgradeService == nil {
		return errors.NotValidf("nil GetUpgradeService")
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
	inputs := []string{
		config.AgentName,
		config.APICallerName,
		config.UpgradeStepsGateName,
		config.DomainServicesName,
	}
	return dependency.Manifold{
		Inputs: inputs,
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

			// Service factory is used to get the upgrade service and
			// then we can locate all the model uuids.
			upgradeService, err := config.GetUpgradeService(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			return config.NewControllerWorker(
				upgradeStepsLock,
				agent,
				apiCaller,
				upgradeService,
				config.PreUpgradeSteps,
				config.UpgradeSteps,
				statusSetter,
				config.Logger,
				config.Clock,
			)
		},
	}
}

// GetUpgradeService retrieves the UpgradeService from the dependency getter.
func GetUpgradeService(getter dependency.Getter, name string) (UpgradeService, error) {
	return coredependency.GetDependencyByName(getter, name, func(a services.ControllerDomainServices) UpgradeService {
		return a.Upgrade()
	})
}
