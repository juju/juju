// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/keyupdater"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig engine.AgentAPIManifoldConfig

// Manifold returns a dependency manifold that runs a authenticationworker worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig(config)

	return engine.AgentAPIManifold(typedConfig, newWorker)
}

func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	w, err := NewWorker(keyupdater.NewState(apiCaller), a.CurrentConfig())
	if err != nil {
		return nil, errors.Annotate(err, "cannot start ssh auth-keys updater worker")
	}
	return w, nil
}
