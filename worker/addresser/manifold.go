// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by the addresser worker.
type ManifoldConfig engine.ApiManifoldConfig

// Manifold returns a Manifold that encapsulates the addresser worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return engine.ApiManifold(
		engine.ApiManifoldConfig(config),
		manifoldStart,
	)
}

// manifoldStart creates an addresser worker, given a base.APICaller.
func manifoldStart(apiCaller base.APICaller) (worker.Worker, error) {
	api := addresser.NewAPI(apiCaller)
	w, err := NewWorker(api)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
