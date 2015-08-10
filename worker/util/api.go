// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// Some (hopefully growing number of) manifolds completely depend on an API
// connection; this type configures them.
type ApiManifoldConfig struct {
	APICallerName string
}

// ApiStartFunc encapsulates the behaviour that varies among ApiManifolds.
type ApiStartFunc func(base.APICaller) (worker.Worker, error)

// ApiManifold returns a dependency.Manifold that calls the supplied start
// func with the API resource defined in the config (once it's present).
func ApiManifold(config ApiManifoldConfig, start ApiStartFunc) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			return start(apiCaller)
		},
	}
}
