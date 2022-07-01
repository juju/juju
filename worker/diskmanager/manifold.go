// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/v3/agent"
	apidiskmanager "github.com/juju/juju/v3/api/agent/diskmanager"
	"github.com/juju/juju/v3/api/base"
	"github.com/juju/juju/v3/cmd/jujud/agent/engine"
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
