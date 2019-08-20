// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package meterstatus provides a worker that executes the meter-status-changed hook
// periodically.
package meterstatus

import (
	"path"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/meterstatus"
	"github.com/juju/juju/core/machinelock"
)

var (
	logger = loggo.GetLogger("juju.worker.meterstatus")
)

// ManifoldConfig identifies the resource names upon which the status manifold depends.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	MachineLock   machinelock.Lock
	Clock         clock.Clock

	NewHookRunner           func(names.UnitTag, machinelock.Lock, agent.Config, clock.Clock) HookRunner
	NewMeterStatusAPIClient func(base.APICaller, names.UnitTag) meterstatus.MeterStatusClient

	NewConnectedStatusWorker func(ConnectedConfig) (worker.Worker, error)
	NewIsolatedStatusWorker  func(IsolatedConfig) (worker.Worker, error)
}

// Manifold returns a status manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.Clock == nil {
				return nil, errors.NotValidf("missing Clock")
			}
			if config.MachineLock == nil {
				return nil, errors.NotValidf("missing MachineLock")
			}
			return newStatusWorker(config, context)
		},
	}
}

func newStatusWorker(config ManifoldConfig, context dependency.Context) (worker.Worker, error) {
	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, err
	}

	tag := agent.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected unit tag, got %v", tag)
	}

	agentConfig := agent.CurrentConfig()
	stateFile := NewStateFile(path.Join(agentConfig.DataDir(), "meter-status.yaml"))
	runner := config.NewHookRunner(unitTag, config.MachineLock, agentConfig, config.Clock)

	// If we don't have a valid APICaller, start a meter status
	// worker that works without an API connection.
	var apiCaller base.APICaller
	err := context.Get(config.APICallerName, &apiCaller)
	if errors.Cause(err) == dependency.ErrMissing {
		logger.Tracef("API caller dependency not available, starting isolated meter status worker.")
		cfg := IsolatedConfig{
			Runner:           runner,
			StateFile:        stateFile,
			Clock:            config.Clock,
			AmberGracePeriod: defaultAmberGracePeriod,
			RedGracePeriod:   defaultRedGracePeriod,
			TriggerFactory:   GetTriggers,
		}
		return config.NewIsolatedStatusWorker(cfg)
	} else if err != nil {
		return nil, err
	}
	logger.Tracef("Starting connected meter status worker.")
	status := config.NewMeterStatusAPIClient(apiCaller, unitTag)

	cfg := ConnectedConfig{
		Runner:    runner,
		StateFile: stateFile,
		Status:    status,
	}
	return config.NewConnectedStatusWorker(cfg)
}
