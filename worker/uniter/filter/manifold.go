// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filter

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.AgentApiManifoldConfig

// Manifold returns a dependency manifold that runs an event filter worker, using
// the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.AgentApiManifold(util.AgentApiManifoldConfig(config), newWorker)
	manifold.Output = outputFunc
	return manifold
}

// newWorker should eventually replace NewFilter as the package factory function,
// and the Worker methods should be removed from the Filter interface; at that point
// all tests can be done via the manifold. For now it's just to be patched out for
// the sake of simplified testing.
var newWorker = func(agent agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	// TODO(fwereade): this worker was extracted from uniter, which is why it
	// still uses the uniter facade. This should probably be fixed at some point.
	unitTag, ok := agent.Tag().(names.UnitTag)
	if !ok {
		return nil, fmt.Errorf("expected a unit tag; got %q", agent.Tag())
	}
	uniterFacade := uniter.NewState(apiCaller, unitTag)
	return NewFilter(uniterFacade, unitTag)
}

// outputFunc exposes a *filter as a Filter.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*filter)
	outPointer, _ := out.(*Filter)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker
	return nil
}
