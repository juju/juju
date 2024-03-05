// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig holds the names of the resources used by, and the
// additional dependencies of, an undertaker worker.
type ManifoldConfig struct {
	APICallerName string

	Logger                       Logger
	Clock                        clock.Clock
	NewFacade                    func(base.APICaller) (Facade, error)
	NewWorker                    func(Config) (worker.Worker, error)
	NewCredentialValidatorFacade func(base.APICaller) (common.CredentialAPI, error)
	NewCloudDestroyerFunc        func(context.Context, environs.OpenParams) (environs.CloudDestroyer, error)
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	var apiCaller base.APICaller
	if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	credentialAPI, err := config.NewCredentialValidatorFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		Facade:                facade,
		CredentialAPI:         credentialAPI,
		Logger:                config.Logger,
		Clock:                 config.Clock,
		NewCloudDestroyerFunc: config.NewCloudDestroyerFunc,
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
		},
		Start: config.start,
	}
}
