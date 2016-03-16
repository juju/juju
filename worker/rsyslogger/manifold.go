// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslogger

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logreader"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
}

// Manifold returns a dependency manifold that runs a logger
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			var agent agent.Agent
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			logreaderFacade := logreader.NewAPI(apiCaller)
			return NewRsysWorker(logreaderFacade, agent.CurrentConfig())
		},
	}
}

// newWorker trivially wraps NewLogger to specialise a PostUpgradeManifold.
var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	currentConfig := a.CurrentConfig()
	logreaderFacade := logreader.NewAPI(apiCaller)
	return NewRsysWorker(logreaderFacade, currentConfig)
}
