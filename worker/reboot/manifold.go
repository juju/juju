// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.PostUpgradeManifoldConfig

// Manifold returns a dependency manifold that runs a reboot worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.PostUpgradeManifold(util.PostUpgradeManifoldConfig(config), newWorker)
}

// newWorker trivially wraps NewReboot for use in a util.PostUpgradeManifold.
//
// TODO(mjs) - It's not tested at the moment, because the scaffolding
// necessary is too unwieldy/distracting to introduce at this point.
func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	apiConn, ok := apiCaller.(api.Connection)
	if !ok {
		return nil, errors.New("unable to obtain api.Connection")
	}
	rebootState, err := apiConn.Reboot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	lock, err := cmdutil.HookExecutionLock(cmdutil.DataDir)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := NewReboot(rebootState, a.CurrentConfig(), lock)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start reboot worker")
	}
	return w, nil
}
