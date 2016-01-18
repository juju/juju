// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apidiskmanager "github.com/juju/juju/api/diskmanager"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.PostUpgradeManifoldConfig

// Manifold returns a dependency manifold that runs a diskmanager worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.PostUpgradeManifold(util.PostUpgradeManifoldConfig(config), newWorker)
}

// newWorker trivially wraps NewWorker for use in a util.PostUpgradeManifold.
func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	t := a.CurrentConfig().Tag()
	tag, ok := t.(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("expected MachineTag, got %#v", t)
	}

	api := apidiskmanager.NewState(apiCaller, tag)

	return NewWorker(DefaultListBlockDevices, api), nil
}
