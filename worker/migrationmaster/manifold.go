// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/juju/api/base"
	masterapi "github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig util.ApiManifoldConfig

// Manifold returns a dependency manifold that runs the migration
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := util.ApiManifoldConfig(config)
	return util.ApiManifold(typedConfig, newWorker)
}

// newWorker is a shim to allow New to work with PostUpgradeManifold.
func newWorker(apiCaller base.APICaller) (worker.Worker, error) {
	client := masterapi.NewClient(apiCaller)
	return New(client), nil
}
