// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftnotifier

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/core/raft/notifyproxy"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	raftleasestore "github.com/juju/juju/state/raftlease"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a raft
// worker in a dependency.Engine.
type ManifoldConfig struct {
	StateName string
	RaftName  string

	Logger    Logger
	NewWorker func(Config) (worker.Worker, error)
	NewTarget func(*state.State, raftleasestore.Logger) raftlease.NotifyTarget
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.RaftName == "" {
		return errors.NotValidf("error RaftName")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewTarget == nil {
		return errors.NotValidf("nil NewTarget")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a raft worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var notifyProxy notifyproxy.NotificationProxy
	if err := context.Get(config.RaftName, &notifyProxy); err != nil {
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

	notifyTarget := config.NewTarget(statePool.SystemState(), config.Logger)

	w, err := config.NewWorker(Config{
		Logger:       config.Logger,
		NotifyTarget: notifyTarget,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// NewTarget creates a new lease notify target using the dependencies in a late
// fashion.
func NewTarget(st *state.State, logger raftleasestore.Logger) raftlease.NotifyTarget {
	return st.LeaseNotifyTarget(logger)
}
