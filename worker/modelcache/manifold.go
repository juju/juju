// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcache

import (
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/core/cache"
	workerstate "github.com/juju/juju/worker/state"
)

// Logger describes the logging methods used in this package by the worker.
type Logger interface {
	Tracef(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManifoldConfig holds the information necessary to run an apiserver
// worker in a dependency.Engine.
type ManifoldConfig struct {
	StateName string
	Logger    Logger

	PrometheusRegisterer prometheus.Registerer

	NewWorker func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("missing PrometheusRegisterer")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("missing NewWorker func")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an apiserver
// worker. The manifold outputs an *apiserverhttp.Mux, for other workers
// to register handlers against.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
		},
		Start:  config.start,
		Output: OutputFunc,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	// Get the state pool after grabbing dependencies so we don't need
	// to remember to call Done on it if they're not running yet.
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		Logger:               config.Logger,
		StatePool:            statePool,
		PrometheusRegisterer: config.PrometheusRegisterer,
		Cleanup: func() {
			stTracker.Done()
		},
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	return w, nil
}

// OutputFunc extracts a *cache.Controller from a *cacheWorker.
func OutputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*cacheWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case **cache.Controller:
		*outPointer = inWorker.controller
	default:
		return errors.Errorf("out should be *cache.Controller; got %T", out)
	}
	return nil
}
