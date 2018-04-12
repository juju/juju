// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by the cleanup worker.
type ManifoldConfig struct {
	APICallerName string
	ClockName     string
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	return nil
}

// Manifold returns a Manifold that encapsulates the cleanup worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName, config.ClockName},
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
	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}
	api := cleaner.NewAPI(apiCaller)
	w, err := NewCleaner(api, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
