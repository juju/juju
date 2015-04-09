// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	ApiCallerName string
}

// Manifold returns a dependency manifold that runs a proxy updater worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ApiCallerName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := getResource(config.ApiCallerName, &apiCaller); err != nil {
				return nil, err
			}
			// TODO(fwereade): This shouldn't be an "environment" facade, it
			// should be specific to the proxyupdater and watching for proxy
			// settings changes, not just watching the "environment".
			return New(environment.NewFacade(apiCaller), false), nil
		},
	}
}
