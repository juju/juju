// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/externalcontrollerupdater"
)

// ManifoldConfig describes the resources used by an
// externalcontrollerupdater worker.
type ManifoldConfig struct {
	APICallerName string

	NewExternalControllerWatcherClient NewExternalControllerWatcherClientFunc
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if cfg.NewExternalControllerWatcherClient == nil {
		return errors.NotValidf("nil NewExternalControllerWatcherClient")
	}
	return nil
}

// Manifold returns a Manifold that runs an externalcontrollerupdater worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}
			return manifoldStart(apiCaller, config.NewExternalControllerWatcherClient)
		},
	}
}

// manifoldStart returns a unitassigner worker using the supplied APICaller.
func manifoldStart(
	apiCaller base.APICaller,
	newExternalControllerWatcherClient NewExternalControllerWatcherClientFunc,
) (worker.Worker, error) {
	client := externalcontrollerupdater.New(apiCaller)
	worker, err := New(
		client,
		newExternalControllerWatcherClient,
		clock.WallClock,
		nil,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
