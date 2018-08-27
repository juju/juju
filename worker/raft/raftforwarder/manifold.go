// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
)

// ManifoldConfig holds the resources needed to start a raft forwarder
// worker in a dependency engine.
type ManifoldConfig struct {
	RaftName       string
	CentralHubName string

	RequestTopic string
	Logger       Logger
	NewWorker    func(Config) (worker.Worker, error)
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var r *raft.Raft
	if err := context.Get(config.RaftName, &r); err != nil {
		return nil, errors.Trace(err)
	}
	var hub *pubsub.StructuredHub
	if err := context.Get(config.CentralHubName, &hub); err != nil {
		return nil, errors.Trace(err)
	}
	return config.NewWorker(Config{
		Raft:   r,
		Hub:    hub,
		Logger: config.Logger,
		Topic:  config.RequestTopic,
	})
}

// Manifold builds a dependency.Manifold for running a raftforwarder
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.RaftName,
			config.CentralHubName,
		},
		Start: config.start,
	}
}
