// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/core/globalclock"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a GlobalClockUpdater
// worker in a dependency.Engine.
type ManifoldConfig struct {
	Clock            clock.Clock
	StateName        string
	LeaseManagerName string
	RaftName         string

	NewWorker      func(Config) (worker.Worker, error)
	UpdateInterval time.Duration
	BackoffDelay   time.Duration
	Logger         Logger
}

func (config ManifoldConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.StateName == "" && config.LeaseManagerName == "" {
		return errors.NotValidf("both StateName and LeaseManagerName empty")
	}
	if config.StateName != "" && config.LeaseManagerName != "" {
		return errors.NewNotValid(nil, "only one of StateName and LeaseManagerName can be set")
	}
	if config.StateName != "" && config.RaftName != "" {
		return errors.NewNotValid(nil, "RaftName only valid with LeaseManagerName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.UpdateInterval <= 0 {
		return errors.NotValidf("non-positive UpdateInterval")
	}
	if config.BackoffDelay <= 0 {
		return errors.NotValidf("non-positive BackoffDelay")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a global clock
// updater worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{}
	if config.StateName != "" {
		inputs = append(inputs, config.StateName)
	} else {
		inputs = append(inputs, config.LeaseManagerName)
	}
	if config.RaftName != "" {
		inputs = append(inputs, config.RaftName)
	}
	return dependency.Manifold{
		Inputs: inputs,
		Start:  config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	if config.RaftName != "" {
		// We don't need anything from raft directly, but if it's set
		// ensure it's running before continuing.
		if err := context.Get(config.RaftName, nil); err != nil {
			return nil, errors.Trace(err)
		}
	}

	cleanup := func() error { return nil }
	var updaterFunc func() (globalclock.Updater, error)
	if config.StateName != "" {
		var stTracker workerstate.StateTracker
		if err := context.Get(config.StateName, &stTracker); err != nil {
			return nil, errors.Trace(err)
		}
		statePool, err := stTracker.Use()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cleanup = stTracker.Done
		updaterFunc = statePool.SystemState().GlobalClockUpdater
	} else {

		var updater globalclock.Updater
		if err := context.Get(config.LeaseManagerName, &updater); err != nil {
			return nil, errors.Trace(err)
		}
		updaterFunc = func() (globalclock.Updater, error) {
			return updater, nil
		}
	}

	worker, err := config.NewWorker(Config{
		NewUpdater:     updaterFunc,
		LocalClock:     config.Clock,
		UpdateInterval: config.UpdateInterval,
		BackoffDelay:   config.BackoffDelay,
		Logger:         config.Logger,
	})
	if err != nil {
		cleanup()
		return nil, errors.Trace(err)
	}

	go func() {
		worker.Wait()
		cleanup()
	}()
	return worker, nil
}
