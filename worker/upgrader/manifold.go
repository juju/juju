// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig util.AgentApiManifoldConfig

// Manifold returns a dependency manifold that runs an upgrader
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.AgentApiManifold(util.AgentApiManifoldConfig(config), newWorker)
}

// newWorker wraps NewUpgrader for the convenience of AgentApiManifold. It should
// eventually replace NewUpgrader.
var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	currentConfig := a.CurrentConfig()
	upgraderFacade := upgrader.NewState(apiCaller)
	return NewAgentUpgrader(
		upgraderFacade,
		currentConfig,
		// TODO(fwereade): surely we shouldn't need both currentConfig
		// *and* currentConfig.UpgradedToVersion?
		currentConfig.UpgradedToVersion(),
		// TODO(fwereade): these are unit-agent-specific, and very much
		// unsuitable for use in a machine agent.
		func() bool { return false },
		make(chan struct{}),
	), nil
}
