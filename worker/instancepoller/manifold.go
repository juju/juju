// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig describes the resources used by the instancepoller worker.
type ManifoldConfig util.ApiManifoldConfig

// Manifold returns a Manifold that encapsulates the instancepoller worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.ApiManifold(
		util.ApiManifoldConfig(config),
		manifoldStart,
	)
}

// manifoldStart creates an instancepoller worker, given a base.APICaller.
func manifoldStart(apiCaller base.APICaller) (worker.Worker, error) {
	api := instancepoller.NewAPI(apiCaller)
	w, err := NewWorker(api)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
