// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	masterapi "github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig util.PostUpgradeManifoldConfig

// Manifold returns a dependency manifold that runs the migration
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.PostUpgradeManifold(util.PostUpgradeManifoldConfig(config), newWorker)
}

// newWorker is a shim to allow New to work with PostUpgradeManifold.
func newWorker(_ agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	client := masterapi.NewClient(apiCaller)
	return New(client), nil
}
