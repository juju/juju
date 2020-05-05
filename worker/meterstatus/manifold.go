// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package meterstatus provides a worker that executes the meter-status-changed hook
// periodically.
package meterstatus

import (
	"os"
	"path"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
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
	NewUniterStateAPIClient func(base.FacadeCaller, names.UnitTag) *common.UnitStateAPI

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
	runner := config.NewHookRunner(unitTag, config.MachineLock, agentConfig, config.Clock)
	localStateFile := path.Join(agentConfig.DataDir(), "meter-status.yaml")

	// If we don't have a valid APICaller, start a meter status
	// worker that works without an API connection. Since the worker
	// cannot talk to the controller to persist its state, we will provide
	// it with a disk-backed StateReadWriter and attempt to push the data
	// back to the controller once we get a valid connection.
	var apiCaller base.APICaller
	err := context.Get(config.APICallerName, &apiCaller)
	if errors.Cause(err) == dependency.ErrMissing {
		logger.Tracef("API caller dependency not available, starting isolated meter status worker.")
		cfg := IsolatedConfig{
			Runner:           runner,
			StateReadWriter:  NewDiskBackedState(localStateFile),
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
	stateReadWriter := NewControllerBackedState(
		config.NewUniterStateAPIClient(
			base.NewFacadeCaller(apiCaller, "MeterStatus"),
			unitTag,
		),
	)

	// Check if a local state file exists from a previous isolated worker
	// instance. If one is found, migrate it to the controller and remove
	// it from disk; this doubles as an auto-magic migration step.
	priorState, err := NewDiskBackedState(localStateFile).Read()
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Annotate(err, "reading locally persisted worker state")
	} else if err == nil {
		logger.Infof("detected locally persisted worker state; migrating to the controller")
		if err = stateReadWriter.Write(priorState); err != nil {
			return nil, errors.Trace(err)
		}

		// We can now safely delete the state from disk. It's fine for
		// the deletion attempt to fail; we simply log it as a warning
		// as it's non-fatal.
		if err = os.Remove(localStateFile); err != nil {
			logger.Warningf("unable to remove existing local state file: %v", err)
		}
	}

	cfg := ConnectedConfig{
		Runner:          runner,
		StateReadWriter: stateReadWriter,
		Status:          status,
	}
	return config.NewConnectedStatusWorker(cfg)
}
