// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater

import (
	"io"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a GlobalClockUpdater
// worker in a dependency.Engine.
type ManifoldConfig struct {
	Clock     clock.Clock
	RaftName  string
	StateName string

	FSM            raftlease.ReadOnlyClock
	LeaseLog       io.Writer
	NewWorker      func(Config) (worker.Worker, error)
	NewTarget      func(*state.State, io.Writer, Logger) raftlease.NotifyTarget
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
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.FSM == nil {
		return errors.NotValidf("nil FSM")
	}
	if config.LeaseLog == nil {
		return errors.NotValidf("nil LeaseLog")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewTarget == nil {
		return errors.NotValidf("nil NewTarget")
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
			config.StateName,
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

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	st := statePool.SystemState()

	notifyTarget := config.NewTarget(st, config.LeaseLog, config.Logger)
	w, err := config.NewWorker(Config{
		NewUpdater: func() globalclock.Updater {
			return newUpdater(r, notifyTarget, config.FSM, timeSleeper{}, config.Logger)
		},
		LocalClock:     config.Clock,
		UpdateInterval: config.UpdateInterval,
		Logger:         config.Logger,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// NewTarget is a shim to construct a raftlease.NotifyTarget for testability.
func NewTarget(st *state.State, logFile io.Writer, errorLog Logger) raftlease.NotifyTarget {
	return st.LeaseNotifyTarget(logFile, errorLog)
}

type timeSleeper struct{}

func (timeSleeper) Sleep(d time.Duration) {
	time.Sleep(d)
}
