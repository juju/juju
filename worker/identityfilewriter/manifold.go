// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package identityfilewriter

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	jworker "github.com/juju/juju/worker"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig engine.AgentAPIManifoldConfig

// Manifold returns a dependency manifold that runs an identity file writer worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig(config)
	return engine.AgentAPIManifold(typedConfig, newWorker)
}

// newWorker trivially wraps NewWorker for use in a engine.AgentAPIManifold.
func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	cfg := a.CurrentConfig()

	// Grab the tag and ensure that it's for a controller.
	if !apiagent.IsAllowedControllerTag(cfg.Tag().Kind()) {
		return nil, errors.New("this manifold may only be used inside a machine or controller agent")
	}

	isController, err := apiagent.IsController(apiCaller, cfg.Tag())
	if err != nil {
		return nil, err
	}
	if !isController {
		return nil, dependency.ErrMissing
	}

	return NewWorker(cfg)
}

var NewWorker = func(agentConfig agent.Config) (worker.Worker, error) {
	inner := func(<-chan struct{}) error {
		return agent.WriteSystemIdentityFile(agentConfig)
	}
	return jworker.NewSimpleWorker(inner), nil
}
