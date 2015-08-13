// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logger"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig util.AgentApiManifoldConfig

// Manifold returns a dependency manifold that runs a logger
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.AgentApiManifold(util.AgentApiManifoldConfig(config), newWorker)
}

// newWorker trivially wraps NewLogger to specialise an AgentApiManifold.
var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	currentConfig := a.CurrentConfig()
	loggerFacade := logger.NewState(apiCaller)
	return NewLogger(loggerFacade, currentConfig), nil
}
