// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package meterstatus provides a worker that executes the meter-status-changed hook
// periodically.
package meterstatus

import (
	"path"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/fslock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/meterstatus"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var (
	logger = loggo.GetLogger("juju.worker.meterstatus")
)

// ManifoldConfig identifies the resource names upon which the status manifold depends.
type ManifoldConfig struct {
	AgentName       string
	APICallerName   string
	MachineLockName string

	NewHookRunner           func(names.UnitTag, *fslock.Lock, agent.Config) HookRunner
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
			config.MachineLockName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			return newStatusWorker(config, getResource)
		},
	}
}

func newStatusWorker(config ManifoldConfig, getResource dependency.GetResourceFunc) (worker.Worker, error) {
	var agent agent.Agent
	if err := getResource(config.AgentName, &agent); err != nil {
		return nil, err
	}

	var machineLock *fslock.Lock
	if err := getResource(config.MachineLockName, &machineLock); err != nil {
		return nil, err
	}

	tag := agent.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected unit tag, got %v", tag)
	}

	agentConfig := agent.CurrentConfig()
	stateFile := NewStateFile(path.Join(agentConfig.DataDir(), "meter-status.yaml"))
	runner := config.NewHookRunner(unitTag, machineLock, agentConfig)

	// If we don't have a valid APICaller, start a meter status
	// worker that works without an API connection.
	var apiCaller base.APICaller
	err := getResource(config.APICallerName, &apiCaller)
	if errors.Cause(err) == dependency.ErrMissing {
		logger.Tracef("API caller dependency not available, starting isolated meter status worker.")
		cfg := IsolatedConfig{
			Runner:           runner,
			StateFile:        stateFile,
			Clock:            clock.WallClock,
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
