// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by metrics workers.
type ManifoldConfig engine.ApiManifoldConfig

// Manifold returns a Manifold that encapsulates various metrics workers.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return engine.ApiManifold(
		engine.ApiManifoldConfig(config),
		manifoldStart,
	)
}

// manifoldStart creates a runner for the metrics workers, given a base.APICaller.
func manifoldStart(apiCaller base.APICaller) (worker.Worker, error) {
	client, err := metricsmanager.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := newMetricsManager(client, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
