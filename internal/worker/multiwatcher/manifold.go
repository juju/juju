// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/multiwatcher"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
)

// Clock describes the time methods used in this package by the worker.
type Clock interface {
	Now() time.Time
}

// ManifoldConfig holds the information necessary to run a model cache worker in
// a dependency.Engine.
type ManifoldConfig struct {
	StateName string
	Clock     Clock
	Logger    logger.Logger

	// NOTE: what metrics do we want to expose here?
	// loop restart count for one.
	PrometheusRegisterer prometheus.Registerer

	NewWorker     func(Config) (worker.Worker, error)
	NewAllWatcher func(*state.StatePool) (state.AllWatcherBacking, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
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
	if config.NewAllWatcher == nil {
		return errors.NotValidf("missing NewAllWatcher func")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a model cache
// worker. The manifold outputs a *cache.Controller, primarily for
// the apiserver to depend on and use.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
		},
		Start:  config.start,
		Output: WorkerFactory,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	pool, _, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	allWatcher, err := config.NewAllWatcher(pool)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		Clock:                config.Clock,
		Logger:               config.Logger,
		Backing:              allWatcher,
		PrometheusRegisterer: config.PrometheusRegisterer,
		Cleanup:              func() { _ = stTracker.Done() },
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return w, nil
}

// WorkerFactory extracts a Factory from a *Worker.
func WorkerFactory(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*Worker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *multiwatcher.Factory:
		// The worker itself is the factory.
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be *multiwatcher.Factory; got %T", out)
	}
	return nil
}
