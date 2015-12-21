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

// AgentApiUpgradesStartFunc encapsulates the behaviour that varies among PostUpgradeManifolds.
type PostUpgradeStartFunc func(agent.Agent, base.APICaller) (worker.Worker, error)

// PostUpgradeManifold returns a dependency.Manifold that calls the
// supplied start func with API and agent resources once machine agent
// upgrades have completed (and all required resources are present).
func PostUpgradeManifold(config PostUpgradeManifoldConfig, start PostUpgradeStartFunc) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.UpgradeWaiterName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var upgradesDone bool
			if err := getResource(config.UpgradeWaiterName, &upgradesDone); err != nil {
				return nil, err
			}
			if !upgradesDone {
				return nil, dependency.ErrMissing
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
