// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/internal/socketlistener"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
)

// SocketListener describes a worker that listens on a unix socket.
type SocketListener interface {
	worker.Worker
}

func NewSocketListener(config socketlistener.Config) (SocketListener, error) {
	return socketlistener.NewSocketListener(config)
}

// ManifoldConfig describes the dependencies required by the controlsocket worker.
type ManifoldConfig struct {
	StateName         string
	Logger            Logger
	NewWorker         func(Config) (worker.Worker, error)
	NewSocketListener func(socketlistener.Config) (SocketListener, error)
	SocketName        string
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
	if cfg.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	if cfg.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (_ worker.Worker, err error) {
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err = getter.Get(cfg.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	var st *state.State
	_, st, err = stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Make sure we clean up state objects if an error occurs.
	defer func() {
		if err != nil {
			_ = stTracker.Done()
		}
	}()

	var w worker.Worker
	w, err = cfg.NewWorker(Config{
		State:             stateShim{st},
		Logger:            cfg.Logger,
		SocketName:        cfg.SocketName,
		NewSocketListener: cfg.NewSocketListener,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}
