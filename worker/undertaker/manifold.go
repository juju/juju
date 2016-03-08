// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/utils/clock"
)

type ManifoldConfig struct {
	APICallerName string
	EnvironName   string
	ClockName     string
	RemoveDelay   time.Duration

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(getResource dependency.GetResourceFunc) (worker.Worker, error) {

	var apiCaller base.APICaller
	if err := getResource(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var environ environs.Environ
	if err := getResource(config.EnvironName, &environ); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := getResource(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		Facade:      facade,
		Environ:     environ,
		Clock:       clock,
		RemoveDelay: config.RemoveDelay,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName, config.EnvironName},
		Start:  config.start,
	}
}
