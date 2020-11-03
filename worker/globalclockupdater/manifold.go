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
)

// ManifoldConfig holds the information necessary to run a GlobalClockUpdater
// worker in a dependency.Engine.
type ManifoldConfig struct {
	Clock            clock.Clock
	LeaseManagerName string
	RaftName         string

	NewWorker      func(Config) (worker.Worker, error)
	UpdateInterval time.Duration
	Logger         Logger
}

func (config ManifoldConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if config.RaftName == "" {
		return errors.NotValidf("empty RaftName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.UpdateInterval <= 0 {
		return errors.NotValidf("non-positive UpdateInterval")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a global clock
// updater worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{config.LeaseManagerName, config.RaftName}
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

	// This enforces a dependency on the Raft forwarder,
	// effectively ensuring this worker is only active on the Raft leader.
	if err := context.Get(config.RaftName, nil); err != nil {
		return nil, errors.Trace(err)
	}

	var updater globalclock.Updater
	if err := context.Get(config.LeaseManagerName, &updater); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		NewUpdater: func() (globalclock.Updater, error) {
			return updater, nil
		},
		LocalClock:     config.Clock,
		UpdateInterval: config.UpdateInterval,
		Logger:         config.Logger,
	})
	return w, errors.Trace(err)
}
