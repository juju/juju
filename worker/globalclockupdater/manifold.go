// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/master"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a GlobalClockUpdater
// worker in a dependency.Engine.
type ManifoldConfig struct {
	ClockName string
	StateName string

	Duration time.Duration
}

// Manifold returns a dependency.Manifold that will run a GlobalClockUpdater.
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
	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	st, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	stMachine, err := st.Machine(machineTag.Id())
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	conn := &Conn{st.MongoSession(), stMachine}

	flag, err := master.NewFlagWorker(master.FlagConfig{
		Clock:    clock,
		Conn:     conn,
		Duration: config.Duration,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}

	go func() {
		flag.Wait()
		stTracker.Done()
	}()
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
	if errors.Cause(err) == master.ErrRefresh {
		err = dependency.ErrBounce
	}
	return err
}
