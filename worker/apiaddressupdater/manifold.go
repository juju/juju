// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/caasoperator"
	"github.com/juju/juju/api/machiner"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig engine.AgentAPIManifoldConfig

// Manifold returns a dependency manifold that runs an API address updater worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig(config)
	return engine.AgentAPIManifold(typedConfig, newWorker)
}

// newWorker trivially wraps NewAPIAddressUpdater for use in a engine.AgentAPIManifold.
// It's not tested at the moment, because the scaffolding necessary is too
// unwieldy/distracting to introduce at this point.
var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	tag := a.CurrentConfig().Tag()

	// TODO(fwereade): use appropriate facade!
	var facade APIAddresser
	switch apiTag := tag.(type) {
	case names.UnitTag:
		facade = uniter.NewState(apiCaller, apiTag)
	case names.ApplicationTag:
		facade = caasoperator.NewClient(apiCaller)
	case names.MachineTag:
		facade = machiner.NewState(apiCaller)
	default:
		return nil, errors.Errorf("expected a unit or machine tag; got %q", tag)
	}

	setter := agent.APIHostPortsSetter{Agent: a}
	w, err := NewAPIAddressUpdater(facade, setter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
