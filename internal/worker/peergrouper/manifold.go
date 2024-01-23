// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
)

// ManifoldConfig holds the information necessary to run a peergrouper
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName string
	ClockName string
	StateName string
	Hub       Hub

	PrometheusRegisterer prometheus.Registerer
	NewWorker            func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a peergrouper.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ClockName,
			config.StateName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := getter.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	_, st, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	mongoSession := st.MongoSession()
	agentConfig := agent.CurrentConfig()
	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	supportsHA := model.Type() != state.ModelTypeCAAS

	w, err := config.NewWorker(Config{
		State:                StateShim{st},
		MongoSession:         MongoSessionShim{mongoSession},
		Clock:                clock,
		Hub:                  config.Hub,
		MongoPort:            controllerConfig.StatePort(),
		APIPort:              controllerConfig.APIPort(),
		ControllerAPIPort:    controllerConfig.ControllerAPIPort(),
		SupportsHA:           supportsHA,
		PrometheusRegisterer: config.PrometheusRegisterer,
		// On machine models, the controller id is the same as the machine/agent id.
		// TODO(wallyworld) - revisit when we add HA to k8s.
		ControllerId: agentConfig.Tag().Id,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}
