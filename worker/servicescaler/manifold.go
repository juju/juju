// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicescaler

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig holds dependencies and configuration for a
// servicescaler worker.
type ManifoldConfig struct {
	APICallerName string
	NewFacade     func(base.APICaller) (Facade, error)
	NewWorker     func(Config) (worker.Worker, error)
}

// start is a method on ManifoldConfig because that feels a bit cleaner
// than closing over config in Manifold.
func (config ManifoldConfig) start(apiCaller base.APICaller) (worker.Worker, error) {
	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return config.NewWorker(Config{
		Facade: facade,
	})
}

// Manifold returns a dependency.Manifold that runs a servicescaler worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.ApiManifold(
		util.ApiManifoldConfig{config.APICallerName},
		config.start,
	)
}
