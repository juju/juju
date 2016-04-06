// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	minionapi "github.com/juju/juju/api/migrationminion"
	"github.com/juju/juju/cmd/jujud/agent/util"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig util.AgentApiManifoldConfig

// Manifold returns a dependency manifold that runs the migration
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := util.AgentApiManifoldConfig(config)
	return util.AgentApiManifold(typedConfig, newWorker)
}

// newWorker is a shim to allow New to work with AgentApiManifold.
func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	client := minionapi.NewClient(apiCaller)
	return New(client, a)
}
