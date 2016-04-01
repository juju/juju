// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logsender"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	APICallerName string
	LogSource     LogRecordCh
}

// Manifold returns a dependency manifold that runs a logger
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := util.ApiManifoldConfig{
		APICallerName: config.APICallerName,
	}
	return util.ApiManifold(typedConfig, config.newWorker)
}

func (config ManifoldConfig) newWorker(apiCaller base.APICaller) (worker.Worker, error) {
	return New(config.LogSource, logsender.NewAPI(apiCaller)), nil
}
