// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one passed as manifold config.
var logger interface{}

// ManifoldConfig holds the information necessary to run a FlagWorker in
// a dependency.Engine.
type ManifoldConfig struct {
	Clock         clock.Clock
	APICallerName string
	Duration      time.Duration
	// TODO(controlleragent) - claimaint should be a ControllerAgentTag
	Claimant names.Tag
	Entity   names.Tag

	NewFacade func(base.APICaller, names.Tag, names.Tag) (Facade, error)
	NewWorker func(FlagConfig) (worker.Worker, error)
}

// Validate ensures the required values are set.
func (config *ManifoldConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("missing APICallerName")
	}
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
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
		Clock:    config.Clock,
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
