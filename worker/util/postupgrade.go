// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// Many machine agent manifolds depend on just the agent, an API
// connection and upgrades being complete; this type configures them.
type PostUpgradeManifoldConfig struct {
	AgentName         string
	APICallerName     string
	UpgradeWaiterName string
}

// UpgradeWaitNotRequired can be passed as the UpgradeWaiterName in
// the config if the manifold shouldn't wait for upgrades to
// complete. This is useful for manifolds that need to run in both the
// unit agent and machine agent.
const UpgradeWaitNotRequired = "-"

// AgentApiUpgradesStartFunc encapsulates the behaviour that varies among PostUpgradeManifolds.
type PostUpgradeStartFunc func(agent.Agent, base.APICaller) (worker.Worker, error)

// PostUpgradeManifold returns a dependency.Manifold that calls the
// supplied start func with API and agent resources once machine agent
// upgrades have completed (and all required resources are present).
//
// The wait for upgrade completion can be skipped if
// UpgradeWaitNotRequired is passed as the UpgradeWaiterName.
func PostUpgradeManifold(config PostUpgradeManifoldConfig, start PostUpgradeStartFunc) dependency.Manifold {
	inputs := []string{
		config.AgentName,
		config.APICallerName,
	}
	if config.UpgradeWaiterName != UpgradeWaitNotRequired {
		inputs = append(inputs, config.UpgradeWaiterName)
	}
	return dependency.Manifold{
		Inputs: inputs,
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			if config.UpgradeWaiterName != UpgradeWaitNotRequired {
				var upgradesDone bool
				if err := getResource(config.UpgradeWaiterName, &upgradesDone); err != nil {
					return nil, err
				}
				if !upgradesDone {
					return nil, dependency.ErrMissing
				}
			}

			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			return start(agent, apiCaller)
		},
	}
}
