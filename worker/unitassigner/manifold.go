// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/unitassigner"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Tracef(string, ...interface{})
}

// ManifoldConfig describes the resources used by a unitassigner worker.
type ManifoldConfig struct {
	APICallerName string
	Logger        Logger
}

// Manifold returns a Manifold that runs a unitassigner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return engine.APIManifold(
		engine.APIManifoldConfig{
			APICallerName: config.APICallerName,
		},
		config.start,
	)
}

// start returns a unitassigner worker using the supplied APICaller.
func (c *ManifoldConfig) start(apiCaller base.APICaller) (worker.Worker, error) {
	facade := unitassigner.New(apiCaller)
	worker, err := New(facade, c.Logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
