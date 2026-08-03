// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/sshsession"
	"github.com/juju/juju/api/base"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/internal/errors"
)

// ManifoldConfig holds the information necessary to run the sshsession worker
// in a dependency.Engine.
type ManifoldConfig struct {
	// AgentName is the agent dependency name.
	AgentName string
	// APICallerName is the api caller dependency name.
	APICallerName string
	// AuthenticationWorkerName is the dependency name of the worker that
	// manages the machine's authorized_keys file, including ephemeral keys.
	AuthenticationWorkerName string
	// Logger is the logger to use for the worker.
	Logger logger.Logger
	// NewWorker creates a new sshsession worker.
	NewWorker func(WorkerConfig) (worker.Worker, error)
	// NewFacadeClient creates the facade client from an API caller.
	NewFacadeClient func(base.APICaller) FacadeClient
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.Errorf("empty AgentName").Add(coreerrors.NotValid)
	}
	if config.APICallerName == "" {
		return errors.Errorf("empty APICallerName").Add(coreerrors.NotValid)
	}
	if config.AuthenticationWorkerName == "" {
		return errors.Errorf("empty AuthenticationWorkerName").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	if config.NewWorker == nil {
		return errors.Errorf("nil NewWorker").Add(coreerrors.NotValid)
	}
	if config.NewFacadeClient == nil {
		return errors.Errorf("nil NewFacadeClient").Add(coreerrors.NotValid)
	}
	return nil
}

// Manifold returns a dependency.Manifold that runs the sshsession worker. The
// manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.AuthenticationWorkerName,
		},
		Start: config.start,
	}
}

func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	var thisAgent agent.Agent
	if err := getter.Get(config.AgentName, &thisAgent); err != nil {
		return nil, errors.Capture(err)
	}

	machineTag, ok := thisAgent.CurrentConfig().Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("expected a machine tag, got %T", thisAgent.CurrentConfig().Tag())
	}

	var apiCaller base.APICaller
	if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Capture(err)
	}

	var ephemeralKeysUpdater coressh.EphemeralKeysUpdater
	if err := getter.Get(config.AuthenticationWorkerName, &ephemeralKeysUpdater); err != nil {
		return nil, errors.Capture(err)
	}

	w, err := config.NewWorker(WorkerConfig{
		Logger:               config.Logger,
		MachineName:          machineTag.Id(),
		FacadeClient:         config.NewFacadeClient(apiCaller),
		EphemeralKeysUpdater: ephemeralKeysUpdater,
		ConnectionDialer:     newConnectionDialer(config.Logger),
	})
	if err != nil {
		return nil, errors.Errorf("cannot start sshsession worker: %w", err)
	}
	return w, nil
}

// NewFacadeClient returns a FacadeClient backed by the SSHSession API facade.
func NewFacadeClient(apiCaller base.APICaller) FacadeClient {
	return sshsession.NewClient(apiCaller)
}
