// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig holds the names of the resources used by, and the
// additional dependencies of, an undertaker worker.
type ManifoldConfig struct {
	APICallerName      string
	CloudDestroyerName string

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	var destroyer environs.CloudDestroyer
	if err := context.Get(config.CloudDestroyerName, &destroyer); err != nil {
		return nil, errors.Trace(err)
	}

	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	worker, err := config.NewWorker(Config{
		Facade:    facade,
		Destroyer: destroyer,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency.Manifold that runs a worker responsible
// for shepherding a Dying model into Dead and ultimate removal.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.CloudDestroyerName,
		},
		Start: config.start,
	}
}
