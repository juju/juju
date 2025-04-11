// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	tunnelerClient "github.com/juju/juju/api/controller/sshtunneler"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/state"
)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	AgentName string

	APICallerName string

	ClockName string
}

// Manifold returns a manifold whose worker contain an SSH tunnel tracker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{
		config.AgentName,
		config.APICallerName,
		config.ClockName,
	}
	return dependency.Manifold{
		Inputs: inputs,
		Output: outputFunc,
		Start:  config.start,
	}
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, err
	}

	agentTag := agent.CurrentConfig().Tag()
	machineTag, ok := agentTag.(names.MachineTag)
	if !ok {
		return nil, errors.NotValidf("machine tag %v", agentTag)
	}

	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, err
	}

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, err
	}

	client := tunnelerClient.NewClient(apiCaller)

	adapterClient := &adapterClient{
		machineTag: machineTag,
		client:     client,
	}

	return NewWorker(adapterClient, clock)
}

// outputFunc extracts a tunnel tracker from a sshTunnelerWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*sshTunnelerWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case **sshtunneler.Tracker:
		*outPointer = inWorker.tunnelTracker
	default:
		return errors.Errorf("out should be sshtunneler.TunnelTracker; got %T", out)
	}
	return nil
}

// adapterClient is a wrapper around the tunnelerClient.Client
// to implement the FacadeClient interface.
// We pass the machine tag of the current controller machine
// to the tunnelerClient.Client to fetch its address.
type adapterClient struct {
	machineTag names.MachineTag
	client     *tunnelerClient.Client
}

// InsertSSHConnRequest implements FacadeClient.
// It inserts a SSH connection request into the state.
func (w *adapterClient) InsertSSHConnRequest(arg state.SSHConnRequestArg) error {
	return w.client.InsertSSHConnRequest(arg)
}

// Addresses implements FacadeClient.
// It returns the public addresses of the current controller machine.
func (w *adapterClient) Addresses() (network.SpaceAddresses, error) {
	return w.client.ControllerAddresses(w.machineTag)
}

// MachineHostKeys implements FacadeClient.
// It returns the host keys for a specified machine.
func (w *adapterClient) MachineHostKeys(modelUUID, machineID string) ([]string, error) {
	mt := names.NewMachineTag(machineID)
	return w.client.MachineHostKeys(modelUUID, mt)
}
