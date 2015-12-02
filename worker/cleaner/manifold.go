// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig describes the resources used by the cleanup worker.
type ManifoldConfig util.ApiManifoldConfig

// Manifold returns a Manifold that encapsulates the cleanup worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.ApiManifold(
		util.ApiManifoldConfig(config),
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
