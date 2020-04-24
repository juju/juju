// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package restorewatcher

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/state"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a restorewatcher
// in a dependency.Engine.
type ManifoldConfig struct {
	StateName string
	NewWorker func(Config) (RestoreStatusWorker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// RestoreStatusWorker is a worker that provides a means of observing
// the restore status.
type RestoreStatusWorker interface {
	worker.Worker

	// RestoreStatus returns the most recently observed restore status.
	RestoreStatus() state.RestoreStatus
}

// Manifold returns a dependency.Manifold that will run a restorewatcher.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.StateName},
		Start:  config.start,
		Output: manifoldOutput,
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
		RestoreInfoWatcher: RestoreInfoWatcherShim{st},
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	return &cleanupWorker{
		RestoreStatusWorker: w,
		cleanup:             func() { stTracker.Done() },
	}, nil
}

func manifoldOutput(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*cleanupWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}
	outf, ok := out.(*func() state.RestoreStatus)
	if !ok {
		return errors.Errorf("out should have type %T; got %T", outf, out)
	}
	*outf = inWorker.RestoreStatus
	return nil
}

type cleanupWorker struct {
	RestoreStatusWorker
	cleanupOnce sync.Once
	cleanup     func()
}

func (w *cleanupWorker) Wait() error {
	err := w.RestoreStatusWorker.Wait()
	w.cleanupOnce.Do(w.cleanup)
	return err
}
