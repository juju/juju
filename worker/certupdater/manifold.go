// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	jujuagent "github.com/juju/juju/agent"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a certupdater
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName                string
	AuthorityName            string
	StateName                string
	NewWorker                func(Config) worker.Worker
	NewMachineAddressWatcher func(st *state.State, machineId string) (AddressWatcher, error)
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
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewMachineAddressWatcher == nil {
		return errors.NotValidf("nil NewMachineAddressWatcher")
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

	st := statePool.SystemState()
	addressWatcher, err := config.NewMachineAddressWatcher(st, agentConfig.Tag().Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := config.NewWorker(Config{
		AddressWatcher:     addressWatcher,
		Authority:          authority,
		APIHostPortsGetter: st,
	})
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// NewMachineAddressWatcher is the function that non-test code should
// pass into ManifoldConfig.NewMachineAddressWatcher.
func NewMachineAddressWatcher(st *state.State, machineId string) (AddressWatcher, error) {
	return st.Machine(machineId)
}
