// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/reboot"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/machinelock"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	MachineLock   machinelock.Lock
}

// Manifold returns a dependency manifold that runs a reboot worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			if config.MachineLock == nil {
				return nil, errors.NotValidf("missing MachineLock")
			}
			return newWorker(agent, apiCaller, config.MachineLock)
		},
	}
}

// newWorker starts a new reboot worker.
//
// TODO(mjs) - It's not tested at the moment, because the scaffolding
// necessary is too unwieldy/distracting to introduce at this point.
func newWorker(a agent.Agent, apiCaller base.APICaller, machineLock machinelock.Lock) (worker.Worker, error) {
	apiConn, ok := apiCaller.(api.Connection)
	if !ok {
		return nil, errors.New("unable to obtain api.Connection")
	}
	rebootState, err := reboot.NewFromConnection(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := NewReboot(rebootState, a.CurrentConfig().Tag(), machineLock)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start reboot worker")
	}
	return w, nil
}
