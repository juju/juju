// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	jujuagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Criticalf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// ManifoldConfig holds the information necessary to run a certupdater
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName                  string
	AuthorityName              string
	StateName                  string
	ChangeStreamName           string
	Logger                     Logger
	NewWorker                  func(Config) (worker.Worker, error)
	NewMachineAddressWatcher   func(st *state.State, machineId string) (AddressWatcher, error)
	NewControllerConfigService func(getter changestream.WatchableDBGetter, logger Logger) ControllerConfigService
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.AuthorityName == "" {
		return errors.NotValidf("empty AuthorityName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewMachineAddressWatcher == nil {
		return errors.NotValidf("nil NewMachineAddressWatcher")
	}
	if config.NewControllerConfigService == nil {
		return errors.NotValidf("nil NewControllerConfigService")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a pki Authority.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.AuthorityName,
			config.StateName,
			config.ChangeStreamName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent jujuagent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var authority pki.Authority
	if err := context.Get(config.AuthorityName, &authority); err != nil {
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

	agentConfig := agent.CurrentConfig()

	st, err := statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	addressWatcher, err := config.NewMachineAddressWatcher(st, agentConfig.Tag().Id())
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err = context.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Annotate(err, "failed to get the DB getter")
	}

	w, err := config.NewWorker(Config{
		AddressWatcher:     addressWatcher,
		Authority:          authority,
		APIHostPortsGetter: st,
		CtrlConfigService:  config.NewControllerConfigService(dbGetter, config.Logger),
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// NewMachineAddressWatcher is the function that non-test code should
// pass into ManifoldConfig.NewMachineAddressWatcher.
func NewMachineAddressWatcher(st *state.State, machineId string) (AddressWatcher, error) {
	machine, err := st.Machine(machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return machineShim{
		machine: machine,
	}, nil
}

// NewControllerConfigService is the function that non-test code should
// pass into ManifoldConfig.NewControllerConfigService.
func NewControllerConfigService(dbGetter changestream.WatchableDBGetter, logger Logger) ControllerConfigService {
	return controllerconfigservice.NewService(
		controllerconfigstate.NewState(coredatabase.NewTxnRunnerFactoryForNamespace(
			dbGetter.GetWatchableDB,
			coredatabase.ControllerNS,
		)),
		domain.NewWatcherFactory(
			func() (changestream.WatchableDB, error) {
				return dbGetter.GetWatchableDB(coredatabase.ControllerNS)
			},
			logger,
		),
	)
}

type machineShim struct {
	machine *state.Machine
}

func (s machineShim) WatchAddresses() watcher.NotifyWatcher {
	return watcherShim{
		watcher: s.machine.WatchAddresses(),
	}
}

func (s machineShim) Addresses() network.SpaceAddresses {
	return s.machine.Addresses()
}

type watcherShim struct {
	watcher state.NotifyWatcher
}

func (s watcherShim) Changes() watcher.NotifyChannel {
	return s.watcher.Changes()
}

func (s watcherShim) Kill() {
	s.watcher.Kill()
}

func (s watcherShim) Wait() error {
	return s.watcher.Wait()
}
