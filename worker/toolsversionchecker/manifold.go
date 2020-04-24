// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/agenttools"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig engine.AgentAPIManifoldConfig

// Manifold returns a dependency manifold that runs a toolsversionchecker worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig(config)
	return engine.AgentAPIManifold(typedConfig, newWorker)
}

func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	tag := a.CurrentConfig().Tag()
	if tag.Kind() != names.MachineTagKind {
		return nil, errors.New("this manifold may only be used inside a machine agent")
	}

	isController, err := apiagent.IsController(apiCaller, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isController {
		return nil, dependency.ErrMissing
	}

	// 4 times a day seems a decent enough amount of checks.
	checkerParams := VersionCheckerParams{
		CheckInterval: time.Hour * 6,
	}
	return New(agenttools.NewFacade(apiCaller), &checkerParams), nil
}
