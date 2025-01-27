// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// Logger holds the methods required to log messages.
type Logger interface {
	Errorf(string, ...interface{})
}

// ManifoldConfig holds the information necessary to run an embedded SSH server
// worker in a dependency.Engine.
type ManifoldConfig struct {
	// StateName holds the name of the state dependency.
	StateName string
	// AgentName holds the name of the agent dependency.
	AgentName string
	// NewServerWrapperWorker is the function that creates the embedded SSH server worker.
	NewServerWrapperWorker func(ServerWrapperWorkerConfig) (worker.Worker, error)
	// NewServerWorker is the function that creates a worker that has a catacomb
	// to run the server and other worker dependencies.
	NewServerWorker func(ServerWorkerConfig, bool) (*ServerWorker, error)
	// Logger is the logger to use for the worker.
	Logger Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.NewServerWrapperWorker == nil {
		return errors.NotValidf("nil NewServerWrapperWorker")
	}
	if config.NewServerWorker == nil {
		return errors.NotValidf("nil NewServerWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an embedded SSH server
// worker. The manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
			config.AgentName,
		},
		Start: config.StartWrapperWorker,
	}
}

// StartWrapperWorker starts the SSH server worker wrapper passing the necessary dependencies.
func (config ManifoldConfig) StartWrapperWorker(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	stateInfo, found := agent.CurrentConfig().StateServingInfo()
	if !found {
		return nil, errors.New("state serving info missing from agent config")
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	statePool, err := stTracker.Use()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	w, err := config.NewServerWrapperWorker(ServerWrapperWorkerConfig{
		StateInfo:       stateInfo,
		StatePool:       statePool,
		NewServerWorker: config.NewServerWorker,
		Logger:          config.Logger,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	return common.NewCleanupWorker(w, func() {
		_ = stTracker.Done()
	}), nil
}
