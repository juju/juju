// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package restorewatcher

import (
	"sync"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/dependency"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a restorewatcher
// in a dependency.Engine.
type ManifoldConfig struct {
	StateName            string
	NewWorker            func(Config) (worker.Worker, error)
	RestoreStatusChanged RestoreStatusChangedFunc
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.RestoreStatusChanged == nil {
		return errors.NotValidf("nil RestoreStatusChanged")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a restorewatcher.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.StateName},
		Start:  config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
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
	w, err := config.NewWorker(Config{
		RestoreInfoWatcher:   RestoreInfoWatcherShim{st},
		RestoreStatusChanged: config.RestoreStatusChanged,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	return &cleanupWorker{
		Worker:  w,
		cleanup: func() { stTracker.Done() },
	}, nil
}

type cleanupWorker struct {
	worker.Worker
	cleanupOnce sync.Once
	cleanup     func()
}

func (w *cleanupWorker) Wait() error {
	err := w.Worker.Wait()
	w.cleanupOnce.Do(w.cleanup)
	return err
}
