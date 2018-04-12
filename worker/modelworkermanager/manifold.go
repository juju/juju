// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/dependency"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a model worker manager
// in a dependency.Engine.
type ManifoldConfig struct {
	StateName      string
	NewWorker      func(Config) (worker.Worker, error)
	NewModelWorker NewModelWorkerFunc
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewModelWorker == nil {
		return errors.NotValidf("nil NewModelWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a model worker manager.
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

	w, err := config.NewWorker(Config{
		ModelWatcher:   statePool.SystemState(),
		ModelGetter:    StatePoolModelGetter{statePool},
		NewModelWorker: config.NewModelWorker,
		ErrorDelay:     jworker.RestartDelay,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { stTracker.Done() }), nil
}
