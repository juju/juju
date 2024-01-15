// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/charmrevisionupdater"
)

// ManifoldConfig describes how to create a worker that checks for updates
// available to deployed charms in an environment.
type ManifoldConfig struct {

	// The named dependencies will be exposed to the start func as resources.
	APICallerName string
	Clock         clock.Clock

	// The remaining dependencies will be used with the resources to configure
	// and create the worker. The period must be greater than 0; the NewFacade
	// and NewWorker fields must not be nil. charmrevision.NewWorker, and
	// NewAPIFacade, are suitable implementations for most clients.
	Period    time.Duration
	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
	Logger    Logger
}

// Manifold returns a dependency.Manifold that runs a charm revision worker
// according to the supplied configuration.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if config.Clock == nil {
				return nil, errors.NotValidf("nil Clock")
			}
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			facade, err := config.NewFacade(apiCaller)
			if err != nil {
				return nil, errors.Annotatef(err, "cannot create facade")
			}

			worker, err := config.NewWorker(Config{
				RevisionUpdater: facade,
				Clock:           config.Clock,
				Period:          config.Period,
				Logger:          config.Logger,
			})
			if err != nil {
				return nil, errors.Annotatef(err, "cannot create worker")
			}
			return worker, nil
		},
	}
}

// NewAPIFacade returns a Facade backed by the supplied APICaller.
var NewAPIFacade = newAPIFacade

func newAPIFacade(apiCaller base.APICaller) (Facade, error) {
	return charmrevisionupdater.NewClient(apiCaller), nil
}

// Facade has all the controller methods used by the charm revision worker.
type Facade interface {
	RevisionUpdater
}
