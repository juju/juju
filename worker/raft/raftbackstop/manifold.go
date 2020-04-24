// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftbackstop

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
)

// ManifoldConfig holds the information necessary to run a worker for
// maintaining the raft backstop configuration in a dependency.Engine.
type ManifoldConfig struct {
	RaftName       string
	CentralHubName string
	AgentName      string

	Logger    loggo.Logger
	NewWorker func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var r *raft.Raft
	if err := context.Get(config.RaftName, &r); err != nil {
		return nil, errors.Trace(err)
	}

	var logStore raft.LogStore
	if err := context.Get(config.RaftName, &logStore); err != nil {
		return nil, errors.Trace(err)
	}
	var hub *pubsub.StructuredHub
	if err := context.Get(config.CentralHubName, &hub); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewWorker(Config{
		Raft:     r,
		LogStore: logStore,
		Hub:      hub,
		Logger:   config.Logger,
		LocalID:  raft.ServerID(agent.CurrentConfig().Tag().Id()),
	})
}

// Manifold returns a dependency.Manifold for running a raftbackstop
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.RaftName,
			config.CentralHubName,
			config.AgentName,
		},
		Start: config.start,
	}
}
