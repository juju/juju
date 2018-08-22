// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dblogpruner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state"
	jworker "github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.dblogpruner")

type Config struct {
	State         *state.State
	Clock         clock.Clock
	PruneInterval time.Duration
}

func (config Config) Validate() error {
	if config.State == nil {
		return errors.NotValidf("nil State")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.PruneInterval <= 0 {
		return errors.NotValidf("non-positive PruneInterval")
	}
	return nil
}

// NewWorker returns a worker which periodically wakes up to remove old log
// entries stored in MongoDB. This worker must not be run in more than one
// agent concurrently.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &pruneWorker{config: config}
	return jworker.NewSimpleWorker(w.loop), nil
}

type pruneWorker struct {
	config Config
}

func (w *pruneWorker) loop(stopCh <-chan struct{}) error {
	controllerConfigWatcher := w.config.State.WatchControllerConfig()
	defer worker.Stop(controllerConfigWatcher)

	var (
		maxLogAge               time.Duration
		maxCollectionMB         int
		controllerConfigChanges = controllerConfigWatcher.Changes()
		pruneTimer              clock.Timer
		pruneCh                 <-chan time.Time
	)

	for {
		select {
		case <-stopCh:
			return tomb.ErrDying

		case _, ok := <-controllerConfigChanges:
			if !ok {
				return errors.New("controller configuration watcher closed")
			}
			controllerConfig, err := w.config.State.ControllerConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load controller configuration")
			}
			newMaxAge := controllerConfig.MaxLogsAge()
			newMaxCollectionMB := controllerConfig.MaxLogSizeMB()
			if newMaxAge != maxLogAge || newMaxCollectionMB != maxCollectionMB {
				logger.Infof("log pruning config: max age: %v, max collection size %dM", newMaxAge, newMaxCollectionMB)
				maxLogAge = newMaxAge
				maxCollectionMB = newMaxCollectionMB
			}
			if pruneTimer == nil {
				// We defer starting the timer until the
				// controller configuration watcher fires
				// for the first time, and we have correct
				// configuration values for pruning below.
				pruneTimer = w.config.Clock.NewTimer(w.config.PruneInterval)
				pruneCh = pruneTimer.Chan()
				defer pruneTimer.Stop()
			}

		case <-pruneCh:
			pruneTimer.Reset(w.config.PruneInterval)
			minLogTime := time.Now().Add(-maxLogAge)
			if err := state.PruneLogs(w.config.State, minLogTime, maxCollectionMB); err != nil {
				return errors.Trace(err)
			}
		}
	}
}
