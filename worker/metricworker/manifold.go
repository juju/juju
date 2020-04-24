// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Warningf(string, ...interface{})
}

// ManifoldConfig describes the resources used by metrics workers.
type ManifoldConfig struct {
	APICallerName string
	Logger        Logger
}

// Manifold returns a Manifold that encapsulates various metrics workers.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return engine.APIManifold(
		engine.APIManifoldConfig{
			APICallerName: config.APICallerName,
		},
		config.start,
	)
}

// start creates a runner for the metrics workers, given a base.APICaller.
func (c *ManifoldConfig) start(apiCaller base.APICaller) (worker.Worker, error) {
	client, err := metricsmanager.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := newMetricsManager(client, nil, c.Logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
