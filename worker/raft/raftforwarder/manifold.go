// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"
)

// ManifoldConfig holds the resources needed to start a raft forwarder
// worker in a dependency engine.
type ManifoldConfig struct {
	RaftName       string
	CentralHubName string

	RequestTopic         string
	PrometheusRegisterer prometheus.Registerer
	Logger               Logger
	NewWorker            func(Config) (worker.Worker, error)
}

// Validate checks that the config has all the required values.
func (config ManifoldConfig) Validate() error {
	if config.CentralHubName == "" {
		return errors.NotValidf("empty CentralHubName")
	}
	if config.RequestTopic == "" {
		return errors.NotValidf("empty RequestTopic")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var r *raft.Raft
	if err := context.Get(config.RaftName, &r); err != nil {
		return nil, errors.Trace(err)
	}
	var hub *pubsub.StructuredHub
	if err := context.Get(config.CentralHubName, &hub); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		Raft:                 r,
		Hub:                  hub,
		Logger:               config.Logger,
		Topic:                config.RequestTopic,
		PrometheusRegisterer: config.PrometheusRegisterer,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
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
