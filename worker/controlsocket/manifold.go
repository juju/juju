// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig describes the dependencies required by the controlsocket worker.
type ManifoldConfig struct {
	StateName  string
	Logger     Logger
	NewWorker  func(Config) (worker.Worker, error)
	SocketName string
}

// Manifold returns a Manifold that encapsulates the controlsocket worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
		},
		Start: config.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker func")
	}
	if cfg.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(cfg.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	st, err := statePool.SystemState()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	w, err := cfg.NewWorker(Config{
		State:      stateShim{st},
		Logger:     cfg.Logger,
		SocketName: cfg.SocketName,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}
