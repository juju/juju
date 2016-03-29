// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/agenttools"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
	"github.com/juju/names"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.AgentApiManifoldConfig

// Manifold returns a dependency manifold that runs a toolsversionchecker worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := util.AgentApiManifoldConfig(config)
	return util.AgentApiManifold(typedConfig, newWorker)
}

func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	st := apiagent.NewState(apiCaller)
	isMM, err := isModelManager(a, st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isMM {
		return nil, dependency.ErrMissing
	}

	// 4 times a day seems a decent enough amount of checks.
	checkerParams := VersionCheckerParams{
		CheckInterval: time.Hour * 6,
	}
	return New(agenttools.NewFacade(apiCaller), &checkerParams), nil
}

func isModelManager(a agent.Agent, st *apiagent.State) (bool, error) {
	cfg := a.CurrentConfig()

	// Grab the tag and ensure that it's for a machine.
	tag, ok := cfg.Tag().(names.MachineTag)
	if !ok {
		return false, errors.New("this manifold may only be used inside a machine agent")
	}

	entity, err := st.Entity(tag)
	if err != nil {
		return false, err
	}

	for _, job := range entity.Jobs() {
		if job == multiwatcher.JobManageModel {
			return true, nil
		}
	}

	return false, nil
}
