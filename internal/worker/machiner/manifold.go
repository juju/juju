// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent/agent"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/base"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
}

// Manifold returns a dependency manifold that runs a machiner worker, using
// the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			return newWorker(ctx, agent, apiCaller)
		},
	}
}

// newWorker non-trivially wraps NewMachiner to specialise a engine.AgentAPIManifold.
//
// TODO(waigani) This function is currently covered by functional tests
// under the machine agent. Add unit tests once infrastructure to do so is
// in place.
func newWorker(ctx context.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	currentConfig := a.CurrentConfig()

	// TODO(fwereade): this functionality should be on the
	// machiner facade instead -- or, better yet, separate
	// the networking concerns from the lifecycle ones and
	// have completely separate workers.
	//
	// (With their own facades.)
	agentFacade, err := apiagent.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelConfig, err := agentFacade.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot read environment config: %v", err)
	}

	ignoreMachineAddresses, _ := modelConfig.IgnoreMachineAddresses()
	// Containers only have machine addresses, so we can't ignore them.
	tag := currentConfig.Tag()
	if names.IsContainerMachine(tag.Id()) {
		ignoreMachineAddresses = false
	}
	if ignoreMachineAddresses {
		logger.Infof(context.Background(), "machine addresses not used, only addresses from provider")
	}
	accessor := APIMachineAccessor{apimachiner.NewClient(apiCaller)}
	w, err := NewMachiner(Config{
		MachineAccessor:              accessor,
		Tag:                          tag.(names.MachineTag),
		ClearMachineAddressesOnStart: ignoreMachineAddresses,
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot start machiner worker")
	}
	return w, err
}
