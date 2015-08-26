// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.AgentApiManifoldConfig

// Manifold returns a dependency manifold that runs an API address updater worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.AgentApiManifold(util.AgentApiManifoldConfig(config), newWorker)
}

// newWorker trivially wraps NewAPIAddressUpdater for use in a util.AgentApiManifold.
// It's not tested at the moment, because the scaffolding necessary to test these 5
// lines outweighs them by several times for very little confirmatory power; in the
// long term, all APIAddressUpdaters should be constructed via a manifold, and the
// tests can be updated to reflect that.
var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	// TODO(fwereade): why on *earth* do we use the *uniter* facade for this
	// worker? This code really ought to work anywhere...
	tag := a.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected a unit tag; got %q", tag)
	}
	return NewAPIAddressUpdater(uniter.NewState(apiCaller, unitTag), agent.APIHostPortsSetter{a}), nil
}
