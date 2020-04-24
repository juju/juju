// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apidiskmanager "github.com/juju/juju/api/diskmanager"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig engine.AgentAPIManifoldConfig

// Manifold returns a dependency manifold that runs a diskmanager worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig(config)
	return engine.AgentAPIManifold(typedConfig, newWorker)
}

// newWorker trivially wraps NewWorker for use in a engine.AgentAPIManifold.
func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	t := a.CurrentConfig().Tag()
	tag, ok := t.(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("expected MachineTag, got %#v", t)
	}

	api := apidiskmanager.NewState(apiCaller, tag)

	return NewWorker(DefaultListBlockDevices, api), nil
}
