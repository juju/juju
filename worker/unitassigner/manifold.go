// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/unitassigner"
	"github.com/juju/juju/cmd/jujud/agent/util"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by a unitassigner worker.
type ManifoldConfig util.ApiManifoldConfig

// Manifold returns a Manifold that runs a unitassigner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.ApiManifold(
		util.ApiManifoldConfig(config),
		manifoldStart,
	)
}

// manifoldStart returns a unitassigner worker using the supplied APICaller.
func manifoldStart(apiCaller base.APICaller) (worker.Worker, error) {
	facade := unitassigner.New(apiCaller)
	worker, err := New(facade)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
