// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgraderembedded

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/gate"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	UpgradeStepsGateName string

	NewClient func(base.APICaller) UpgraderClient
	Logger    Logger
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.UpgradeStepsGateName == "" {
		return errors.NotValidf("empty UpgradeStepsGateName")
	}
	if config.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
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

	upgraderFacade := config.NewClient(apiCaller)

	var upgradeStepsWaiter gate.Waiter
	if config.UpgradeStepsGateName == "" {
		upgradeStepsWaiter = gate.NewLock()
	} else {
		if err := context.Get(config.UpgradeStepsGateName, &upgradeStepsWaiter); err != nil {
			return nil, err
		}
	}

	return NewUpgrader(Config{
		UpgraderClient:     upgraderFacade,
		AgentTag:           currentConfig.Tag(),
		UpgradeStepsWaiter: upgradeStepsWaiter,
		Logger:             config.Logger,
	})
}

// Manifold returns a dependency manifold that runs an upgrader
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{
		config.AgentName,
		config.APICallerName,
		config.UpgradeStepsGateName,
	}
	return dependency.Manifold{
		Inputs: inputs,
		Start:  config.start,
	}
}
