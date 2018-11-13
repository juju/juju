// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dblogpruner

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state"
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
	w := &pruneWorker{
		config: config,
	}
	w.tomb.Go(w.loop)
	return w, nil
}

type pruneWorker struct {
	tomb    tomb.Tomb
	mu      sync.Mutex
	config  Config
	current report
}

func (w *pruneWorker) Report() map[string]interface{} {
	w.mu.Lock()
	report := w.current
	w.mu.Unlock()

	// The keys used give a nice output when alphabetical
	// which is how the report yaml gets serialised.
	result := map[string]interface{}{
		"prune-age":  report.maxLogAge,
		"prune-size": report.maxCollectionMB,
	}
	if !report.lastPrune.IsZero() {
		result["last-prune"] = report.lastPrune.Round(time.Second)
	}
	if !report.nextPrune.IsZero() {
		result["next-prune"] = report.nextPrune.Round(time.Second)
	}
	if report.message != "" {
		result["summary"] = report.message
	}
	if report.pruning {
		result["pruning-in-progress"] = true
	}
	return result
}

type reportRequest struct {
	response chan<- report
}

type report struct {
	lastPrune       time.Time
	nextPrune       time.Time
	maxLogAge       time.Duration
	maxCollectionMB int
	message         string
	pruning         bool
}

func (w *pruneWorker) loop() error {
	controllerConfigWatcher := w.config.State.WatchControllerConfig()
	defer worker.Stop(controllerConfigWatcher)

	var prune <-chan time.Time
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case _, ok := <-controllerConfigWatcher.Changes():
			if !ok {
				return errors.New("controller configuration watcher closed")
			}
			controllerConfig, err := w.config.State.ControllerConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load controller configuration")
			}
			newMaxAge := controllerConfig.MaxLogsAge()
			newMaxCollectionMB := controllerConfig.MaxLogSizeMB()
			if newMaxAge != w.current.maxLogAge || newMaxCollectionMB != w.current.maxCollectionMB {
				w.mu.Lock()
				w.current.maxLogAge = newMaxAge
				w.current.maxCollectionMB = newMaxCollectionMB
				w.mu.Unlock()
				logger.Infof("log pruning config: max age: %v, max collection size %dM", newMaxAge, newMaxCollectionMB)
			}
			if prune == nil {
				// We defer starting the timer until the
				// controller configuration watcher fires
				// for the first time, and we have correct
				// configuration values for pruning below.
				prune = w.config.Clock.After(w.config.PruneInterval)
				w.mu.Lock()
				w.current.nextPrune = w.config.Clock.Now().Add(w.config.PruneInterval)
				w.mu.Unlock()
			}

		case <-prune:
			now := w.config.Clock.Now()
			prune = w.config.Clock.After(w.config.PruneInterval)
			w.mu.Lock()
			w.current.lastPrune = now
			w.current.nextPrune = now.Add(w.config.PruneInterval)
			w.current.pruning = true
			w.mu.Unlock()

			minLogTime := now.Add(-w.current.maxLogAge)
			message, err := state.PruneLogs(w.config.State, minLogTime, w.current.maxCollectionMB, logger)
			if err != nil {
				return errors.Trace(err)
			}
			w.mu.Lock()
			w.current.pruning = false
			w.current.message = message
			w.mu.Unlock()
		}
	}
}

// Kill implements Worker.Kill().
func (w *pruneWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait().
func (w *pruneWorker) Wait() error {
	return w.tomb.Wait()
}
