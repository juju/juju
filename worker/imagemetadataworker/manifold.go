// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.PostUpgradeManifoldConfig

// Manifold returns a dependency manifold that runs an image metadata worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.PostUpgradeManifold(util.PostUpgradeManifoldConfig(config), newWorker)
}

// newWorker trivially wraps NewWorker for use in a util.PostUpgradeManifold.
func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {

	st := apiagent.NewState(apiCaller)
	isMM, err := isModelManager(a, st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isMM {
		return nil, dependency.ErrMissing
	}

	hasR, err := hasRegion(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !hasR {
		return nil, dependency.ErrUninstall
	}

	apiConn, ok := apiCaller.(api.Connection)
	if !ok {
		return nil, errors.New("unable to obtain api.Connection")
	}

	// Start worker that stores published image metadata in state.
	return NewWorker(imagemetadata.NewClient(apiConn)), nil
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

func hasRegion(st *apiagent.State) (bool, error) {
	mCfg, err := st.ModelConfig()
	if err != nil {
		return false, errors.Trace(err)
	}

	// Published image metadata for some providers are in simple streams.
	// Providers that do not depend on simple streams do not need this worker.
	env, err := environs.New(mCfg)
	if err != nil {
		return false, errors.Annotate(err, "getting environ")
	}
	if _, ok := env.(simplestreams.HasRegion); ok {
		return true, nil
	}

	return false, nil
}
