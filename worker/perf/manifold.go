// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package perf

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a performance test
// plan.
type ManifoldConfig struct {
	AgentName string
	StateName string

	Clock          clock.Clock
	Logger         Logger
	MetricsStepper MetricStep
}

type MetricStep interface {
	RecordPerfStep()
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.MetricsStepper == nil {
		return errors.NotValidf("nil MetricsStepper")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an HTTP server
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.StateName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(getter dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, err
	}

	currentModelUUID := agent.CurrentConfig().Model().Id()

	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	statePool, err := stTracker.Use()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	systemState, err := statePool.SystemState()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	pooledState, err := statePool.Get(currentModelUUID)
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	w, err := newPerfWorker(currentModelUUID, systemState, pooledState.State, config.Clock, config.Logger, config.MetricsStepper)
	if err != nil {
		_ = pooledState.Release()
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() {
		_ = pooledState.Release()
		_ = stTracker.Done()
	}), nil
}
