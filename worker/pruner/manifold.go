// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/api/base"
)

// ManifoldConfig describes the resources and configuration on which the
// statushistorypruner worker depends.
type ManifoldConfig struct {
	APICallerName string
	Clock         clock.Clock
	PruneInterval time.Duration
	NewWorker     func(Config) (worker.Worker, error)
	NewFacade     func(base.APICaller) Facade
	Logger        Logger
}

// Manifold returns a Manifold that encapsulates the statushistorypruner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName},
		Start:  config.start,
	}
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	facade := config.NewFacade(apiCaller)
	prunerConfig := Config{
		Facade:        facade,
		PruneInterval: config.PruneInterval,
		Clock:         config.Clock,
		Logger:        config.Logger,
	}
	w, err := config.NewWorker(prunerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}
