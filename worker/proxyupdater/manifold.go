// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.ApiManifoldConfig

// Manifold returns a dependency manifold that runs a proxy updater worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.ApiManifold(util.ApiManifoldConfig(config), newWorker)
}

// newWorker is not currently tested; it should eventually replace New as the
// package's exposed factory func, and then all tests should pass through it.
func newWorker(apiCaller base.APICaller) (worker.Worker, error) {
	// TODO(fwereade): This shouldn't be an "environment" facade, it
	// should be specific to the proxyupdater, and be watching for
	// *proxy settings* changes, not just watching the "environment".
	return New(environment.NewFacade(apiCaller), false), nil
}
