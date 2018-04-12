// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources and configuration on which the
// statushistorypruner worker depends.
type ManifoldConfig struct {
	APICallerName string
	EnvironName   string
	ClockName     string
	PruneInterval time.Duration
	NewWorker     func(Config) (worker.Worker, error)
	NewFacade     func(base.APICaller) Facade
}

// Manifold returns a Manifold that encapsulates the statushistorypruner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName, config.EnvironName, config.ClockName},
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

	facade := config.NewFacade(apiCaller)
	prunerConfig := Config{
		Facade:        facade,
		PruneInterval: config.PruneInterval,
		Clock:         clock,
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
	if config.EnvironName == "" {
		return errors.NotValidf("empty EnvironName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	return nil
}
