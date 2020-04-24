// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txnpruner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a txn pruner
// worker in a dependency.Engine.
type ManifoldConfig struct {
	ClockName string
	StateName string

	PruneInterval time.Duration
	NewWorker     func(TransactionPruner, time.Duration, clock.Clock) worker.Worker
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

// Manifold returns a dependency.Manifold that will run a txn pruner
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

	worker := config.NewWorker(statePool.SystemState(), config.PruneInterval, clock)
	go func() {
		worker.Wait()
		stTracker.Done()
	}()
	return worker, nil
}
