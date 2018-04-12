// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig holds the information necessary to run a FlagWorker in
// a dependency.Engine.
type ManifoldConfig struct {
	ClockName     string
	APICallerName string
	Duration      time.Duration
	Claimant      names.MachineTag
	Entity        names.Tag

	NewFacade func(base.APICaller, names.MachineTag, names.Tag) (Facade, error)
	NewWorker func(FlagConfig) (worker.Worker, error)
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}
	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	facade, err := config.NewFacade(apiCaller, config.Claimant, config.Entity)
	if err != nil {
		return nil, errors.Trace(err)
	}
	flag, err := config.NewWorker(FlagConfig{
		Clock:    clock,
		Facade:   facade,
		Duration: config.Duration,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return wrappedWorker{flag}, nil
}

// wrappedWorker wraps a flag worker, translating ErrRefresh into
// dependency.ErrBounce.
type wrappedWorker struct {
	worker.Worker
}

// Wait is part of the worker.Worker interface.
func (w wrappedWorker) Wait() error {
	err := w.Worker.Wait()
	if err == ErrRefresh {
		err = dependency.ErrBounce
	}
	return err
}

// Manifold returns a dependency.Manifold that will run a FlagWorker and
// expose it to clients as a engine.Flag resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ClockName,
			config.APICallerName,
		},
		Start: config.start,
		Output: func(in worker.Worker, out interface{}) error {
			if w, ok := in.(wrappedWorker); ok {
				in = w.Worker
			}
			return engine.FlagOutput(in, out)
		},
	}
}
