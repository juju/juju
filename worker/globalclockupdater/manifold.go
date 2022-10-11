// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/raftlease"
)

// ManifoldConfig holds the information necessary to run a GlobalClockUpdater
// worker in a dependency.Engine.
type ManifoldConfig struct {
	Clock    clock.Clock
	RaftName string

	FSM            raftlease.ReadOnlyClock
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
	if config.FSM == nil {
		return errors.NotValidf("nil FSM")
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
		Inputs: []string{
			config.RaftName,
		},
		Start: config.start,
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
		NewUpdater: func() globalclock.Updater {
			return newUpdater(r, config.FSM, timeSleeper{}, timeTimer{}, config.Logger)
		},
		LocalClock:     config.Clock,
		UpdateInterval: config.UpdateInterval,
		Logger:         config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type timeSleeper struct{}

func (timeSleeper) Sleep(d time.Duration) {
	time.Sleep(d)
}

type timeTimer struct{}

func (timeTimer) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
