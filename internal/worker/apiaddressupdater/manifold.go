// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/agent/caasagent"
	"github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	Logger        logger.Logger
}

// Manifold returns a dependency manifold that runs an API address updater worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return engine.AgentAPIManifold(typedConfig, config.newWorker)
}

// newWorker trivially wraps NewAPIAddressUpdater for use in a engine.AgentAPIManifold.
func (config ManifoldConfig) newWorker(_ context.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	facade, err := newAPIAddresser(a.CurrentConfig().Tag(), apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	setter := agent.APIHostPortsSetter{Agent: a}
	w, err := NewAPIAddressUpdater(Config{
		Addresser: facade,
		Setter:    setter,
		Logger:    config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func newAPIAddresser(tag names.Tag, apiCaller base.APICaller) (APIAddresser, error) {
	switch apiTag := tag.(type) {
	case names.UnitTag:
		return uniter.NewClient(apiCaller, apiTag), nil
	case names.MachineTag:
		return machiner.NewClient(apiCaller), nil
	case names.ControllerAgentTag:
		return caasagent.NewAPIAddressClient(apiCaller), nil
	default:
		return nil, errors.Errorf("expected a unit, machine, or controller agent tag; got %q", tag)
	}
}
