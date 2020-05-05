// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerport

import (
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
	workerstate "github.com/juju/juju/worker/state"
)

// Logger defines the methods needed for the worker to log messages.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManifoldConfig holds the information necessary to determine the controller
// api port and keep it up to date.
type ManifoldConfig struct {
	AgentName string
	HubName   string
	StateName string

	Logger                  Logger
	UpdateControllerAPIPort func(int) error

	GetControllerConfig func(*state.State) (controller.Config, error)
	NewWorker           func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.HubName == "" {
		return errors.NotValidf("empty HubName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.UpdateControllerAPIPort == nil {
		return errors.NotValidf("nil UpdateControllerAPIPort")
	}
	if config.GetControllerConfig == nil {
		return errors.NotValidf("nil GetControllerConfig")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an HTTP server
// worker. The manifold outputs an *apiserverhttp.Mux, for other workers
// to register handlers against.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.HubName,
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

	var hub *pubsub.StructuredHub
	if err := context.Get(config.HubName, &hub); err != nil {
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
	defer stTracker.Done()

	systemState := statePool.SystemState()
	controllerConfig, err := config.GetControllerConfig(systemState)
	if err != nil {
		return nil, errors.Annotate(err, "unable to get controller config")
	}
	controllerAPIPort := controllerConfig.ControllerAPIPort()

	w, err := config.NewWorker(Config{
		AgentConfig:             agent.CurrentConfig(),
		Hub:                     hub,
		Logger:                  config.Logger,
		ControllerAPIPort:       controllerAPIPort,
		UpdateControllerAPIPort: config.UpdateControllerAPIPort,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// GetControllerConfig gets the controller config from the given state
// - it's a shim so we can test the manifold without a state suite.
func GetControllerConfig(st *state.State) (controller.Config, error) {
	return st.ControllerConfig()
}
