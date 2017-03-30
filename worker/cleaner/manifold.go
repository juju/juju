// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"github.com/juju/errors"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by the cleanup worker.
type ManifoldConfig engine.APIManifoldConfig

// Manifold returns a Manifold that encapsulates the cleanup worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return engine.APIManifold(
		engine.APIManifoldConfig(config),
		manifoldStart,
	)
}

// manifoldStart creates a cleaner worker, given a base.APICaller.
func manifoldStart(apiCaller base.APICaller) (worker.Worker, error) {
	api := cleaner.NewAPI(apiCaller)
	w, err := NewCleaner(api)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
