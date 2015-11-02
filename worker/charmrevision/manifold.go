// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes how to create a worker that checks for updates
// available to deployed charms in an environment.
type ManifoldConfig struct {

	// The named dependencies will be exposed to the start func as resources.
	APICallerName string
	ClockName     string

	// The remaining dependencies will be used with the resources to configure
	// and create the worker. The period must be greater than 0; the NewFacade
	// and NewWorker fields must not be nil, but many clients can safely use
	// the NewFacade and NewWorker funcs defined in this package and not worry.
	Period    time.Duration
	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

// Manifold returns a dependency.Manifold that runs a charm revision worker
// according to the supplied configuration.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.ClockName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var clock clock.Clock
			if err := getResource(config.ClockName, &clock); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			facade, err := config.NewFacade(apiCaller)
			if err != nil {
				return nil, errors.Annotatef(err, "cannot create facade")
			}

			worker, err := config.NewWorker(Config{
				Period: config.Period,
				Facade: facade,
				Clock:  clock,
			})
			if err != nil {
				return nil, errors.Annotatef(err, "cannot create worker")
			}
			return worker, nil
		},
	}
}
