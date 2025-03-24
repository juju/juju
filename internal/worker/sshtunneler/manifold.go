// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	tunnelerClient "github.com/juju/juju/api/controller/sshtunneler"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/state"
)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	AgentName string

	APICallerName string

	TunnelerSecret string
}

// Manifold returns a manifold whose worker wraps a JWT parser.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{
		config.AgentName,
		config.APICallerName,
		config.TunnelerSecret,
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

	var tunnelerSecret *sshtunneler.TunnelSecret
	if err := context.Get(config.TunnelerSecret, &tunnelerSecret); err != nil {
		return nil, err
	}

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, err
	}

	client, err := tunnelerClient.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	wrappedClient := &wrappedTunnelerClient{
		machineTag: machineTag,
		client:     client,
	}

	return NewWorker(wrappedClient, tunnelerSecret)
}

// outputFunc extracts a jwtparser.Parser from a
// jwtParserWorker contained within a CleanupWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	inWorker, _ := in.(*sshTunnelerWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case **sshtunneler.TunnelTracker:
		*outPointer = inWorker.tunnelTracker
	default:
		return errors.Errorf("out should be sshtunneler.TunnelTracker; got %T", out)
	}
	return nil
}

type wrappedTunnelerClient struct {
	machineTag names.MachineTag
	client     *tunnelerClient.Client
}

func (w *wrappedTunnelerClient) InsertSSHConnRequest(arg state.SSHConnRequestArg) error {
	return w.client.InsertSSHConnRequest(arg)
}

func (w *wrappedTunnelerClient) Addresses() (network.SpaceAddresses, error) {
	return w.client.ControllerAddresses(w.machineTag)
}
