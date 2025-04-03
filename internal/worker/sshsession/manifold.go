// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/sshsession"
	"github.com/juju/juju/api/base"
)

// Logger holds the methods required to log messages.
type Logger interface {
	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
}

// ManifoldConfig holds the information necessary to run the session
// worker in a dependency.Engine.
type ManifoldConfig struct {
	// APICallerName holds the api caller dependency name.
	APICallerName string

	// AgentName holds the agent dependency name.
	AgentName string

	// Logger is the logger to use for the worker.
	Logger Logger

	// NewWorker returns a new sshsession worker.
	NewWorker func(cfg WorkerConfig) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an embedded SSH server
// worker. The manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.AgentName,
		},
		Start: config.start,
	}
}

// start starts the worker.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, err
	}

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, err
	}

	machineId := agent.CurrentConfig().Tag().Id()

	w, err := config.NewWorker(WorkerConfig{
		Logger:           config.Logger,
		MachineId:        machineId,
		FacadeClient:     sshsession.NewClient(apiCaller),
		ConnectionGetter: NewConnectionGetter(config.Logger),
		KeyManager:       NewKeyManager(config.Logger),
	})

	if err != nil {
		return nil, errors.Annotate(err, "cannot start sshsession worker")
	}

	return w, nil
}
