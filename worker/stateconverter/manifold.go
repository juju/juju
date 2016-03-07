// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	apimachiner "github.com/juju/juju/api/machiner"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/conv2state"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	AgentRestart func() error
}

// Manifold returns a dependency manifold that runs a stateconverter worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {

	// newWorker trivially wraps NewWorker for use in a util.PostUpgradeManifold.
	var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		cfg := a.CurrentConfig()

		// Grab the tag and ensure that it's for a machine.
		tag, ok := cfg.Tag().(names.MachineTag)
		if !ok {
			return nil, errors.New("this manifold may only be used inside a machine agent")
		}

		isMM, err := isModelManager(apiCaller, tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if isMM {
			return nil, dependency.ErrMissing
		}

		handler := conv2state.New(apimachiner.NewState(apiCaller), tag, config.AgentRestart)
		return NewWorker(handler)
	}

	return util.PostUpgradeManifold(config.PostUpgradeManifoldConfig, newWorker)
}

var NewWorker = func(handler watcher.NotifyHandler) (worker.Worker, error) {
	// TODO(fwereade): this worker needs its own facade.
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: handler,
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot start controller promoter worker")
	}
	return w, nil
}

func isModelManager(apiCaller base.APICaller, tag names.Tag) (bool, error) {
	entity, err := apiagent.NewState(apiCaller).Entity(tag)
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
