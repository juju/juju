// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine

import (
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/dependency"
)

// Some (hopefully growing number of) manifolds completely depend on an API
// connection; this type configures them.
type APIManifoldConfig struct {
	APICallerName string
}

// APIStartFunc encapsulates the behaviour that varies among APIManifolds.
type APIStartFunc func(base.APICaller) (worker.Worker, error)

// APIManifold returns a dependency.Manifold that calls the supplied start
// func with the API resource defined in the config (once it's present).
func APIManifold(config APIManifoldConfig, start APIStartFunc) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			return start(apiCaller)
		},
	}
}
