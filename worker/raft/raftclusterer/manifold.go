// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftclusterer

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
)

// ManifoldConfig holds the information necessary to run a worker for
// maintaining the raft cluster configuration in a dependency.Engine.
type ManifoldConfig struct {
	RaftName       string
	CentralHubName string

	NewWorker func(Config) (worker.Worker, error)
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
		Raft: r,
		Hub:  hub,
	})
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.RaftName,
			config.CentralHubName,
		},
		Start: config.start,
	}
}
