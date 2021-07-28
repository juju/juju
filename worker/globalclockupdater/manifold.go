// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/core/globalclock"
)

// ManifoldConfig holds the information necessary to run a GlobalClockUpdater
// worker in a dependency.Engine.
type ManifoldConfig struct {
	Clock    clock.Clock
	RaftName string

	NewWorker      func(Config) (worker.Worker, error)
	UpdateInterval time.Duration
	Logger         Logger
}

func (config ManifoldConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
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
	return dependency.Manifold{
		Inputs: []string{config.RaftName},
		Start:  config.start,
	}
}

// start creates and returns a new clock updater worker based on this config.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var r *raft.Raft
	if err := context.Get(config.RaftName, &r); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		NewUpdater:     func() globalclock.Updater { return &updater{raft: r} },
		LocalClock:     config.Clock,
		UpdateInterval: config.UpdateInterval,
		Logger:         config.Logger,
	})
	return w, errors.Trace(err)
}
