// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type ManifoldConfig struct {
	ApiConnectionName string
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ApiConnectionName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var apiConnection *api.State
			if !getResource(config.ApiConnectionName, &apiConnection) {
				return nil, dependency.ErrUnmetDependencies
			}
			return New(apiConnection.Environment(), false), nil
		},
	}
}
