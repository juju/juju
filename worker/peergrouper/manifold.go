// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a peergrouper
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName          string
	ClockName          string
	ControllerPortName string
	StateName          string
	Hub                Hub

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
	if config.ControllerPortName == "" {
		return errors.NotValidf("empty ControllerPortName")
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
			config.ControllerPortName,
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

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	// Ensure that the controller-port worker is running.
	if err := context.Get(config.ControllerPortName, nil); err != nil {
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
	mongoSession := st.MongoSession()
	agentConfig := agent.CurrentConfig()
	stateServingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return nil, errors.New("state serving info missing from agent config")
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	supportsHA := model.Type() != state.ModelTypeCAAS

	w, err := config.NewWorker(Config{
		State:                StateShim{st},
		MongoSession:         MongoSessionShim{mongoSession},
		APIHostPortsSetter:   &CachingAPIHostPortsSetter{APIHostPortsSetter: st},
		Clock:                clock,
		Hub:                  config.Hub,
		MongoPort:            stateServingInfo.StatePort,
		APIPort:              stateServingInfo.APIPort,
		ControllerAPIPort:    stateServingInfo.ControllerAPIPort,
		SupportsHA:           supportsHA,
		PrometheusRegisterer: config.PrometheusRegisterer,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}
