// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dblogpruner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a log pruner
// worker in a dependency.Engine.
type ManifoldConfig struct {
	ClockName string
	StateName string

	PruneInterval time.Duration
	NewWorker     func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) Validate() error {
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.PruneInterval <= 0 {
		return errors.NotValidf("non-positive PruneInterval")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a log pruner
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ClockName,
			config.StateName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		State:         statePool.SystemState(),
		Clock:         clock,
		PruneInterval: config.PruneInterval,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}

	go func() {
		worker.Wait()
		stTracker.Done()
	}()
	return worker, nil
}
