// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/machineundertaker"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
)

// ManifoldConfig defines the machine undertaker's configuration and
// dependencies.
type ManifoldConfig struct {
	APICallerName string
	EnvironName   string
	Logger        logger.Logger

	NewWorker func(Facade, environs.Environ, logger.Logger) (worker.Worker, error)
}

// Manifold returns a dependency.Manifold that runs a machine
// undertaker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName, config.EnvironName},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			var environ environs.Environ
			if err := getter.Get(config.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}
			api, err := machineundertaker.NewAPI(apiCaller, watcher.NewNotifyWatcher)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(api, environ, config.Logger)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
