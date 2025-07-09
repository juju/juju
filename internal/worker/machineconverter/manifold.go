// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineconverter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent/agent"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// ManifoldConfig provides the dependencies for the
// stateconverter manifold.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	Logger        logger.Logger

	NewMachineClient func(dependency.Getter, string) (MachineClient, error)
	NewAgentClient   func(dependency.Getter, string) (AgentClient, error)
	NewConverter     func(Config) (watcher.NotifyHandler, error)
}

// Manifold returns a Manifold that encapsulates the stateconverter worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.AgentName,
			cfg.APICallerName,
		},
		Start: cfg.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewMachineClient == nil {
		return errors.NotValidf("nil NewMachineClient")
	}
	if cfg.NewAgentClient == nil {
		return errors.NotValidf("nil NewAgentClient")
	}
	if cfg.NewConverter == nil {
		return errors.NotValidf("nil NewConverter")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var a agent.Agent
	if err := getter.Get(cfg.AgentName, &a); err != nil {
		return nil, errors.Trace(err)
	}

	tag := a.CurrentConfig().Tag()
	mTag, ok := tag.(names.MachineTag)
	if !ok {
		return nil, errors.NotValidf("%q machine tag", a)
	}

	machineClient, err := cfg.NewMachineClient(getter, cfg.APICallerName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	agentClient, err := cfg.NewAgentClient(getter, cfg.APICallerName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	handler, err := cfg.NewConverter(Config{
		machineTag:    mTag,
		agent:         a,
		machineClient: machineClient,
		agentClient:   agentClient,
		logger:        cfg.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: handler,
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot start controller promoter worker")
	}
	return w, nil
}

// NewMachineClient returns a new MachineClient that can be used to
// interact with the machiner API.
func NewMachineClient(getter dependency.Getter, apiCallerName string) (MachineClient, error) {
	var apiConn api.Connection
	if err := getter.Get(apiCallerName, &apiConn); err != nil {
		return nil, errors.Trace(err)
	}

	return wrapper{m: apimachiner.NewClient(apiConn)}, nil
}

// NewAgentClient returns a new AgentClient that can be used to
// interact with the agent API.
func NewAgentClient(getter dependency.Getter, apiCallerName string) (AgentClient, error) {
	var apiCaller base.APICaller
	if err := getter.Get(apiCallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	apiState, err := apiagent.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return apiState, nil
}
