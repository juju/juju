// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder

import (
	"io"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	raftleasestore "github.com/juju/juju/state/raftlease"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the resources needed to start a raft forwarder
// worker in a dependency engine.
type ManifoldConfig struct {
	RaftName       string
	StateName      string
	CentralHubName string

	RequestTopic         string
	PrometheusRegisterer prometheus.Registerer
	LeaseLog             io.Writer
	Logger               Logger
	NewWorker            func(Config) (worker.Worker, error)
	NewTarget            func(*state.State, raftleasestore.Logger) raftlease.NotifyTarget
}

// Validate checks that the config has all the required values.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.CentralHubName == "" {
		return errors.NotValidf("empty CentralHubName")
	}
	if config.RequestTopic == "" {
		return errors.NotValidf("empty RequestTopic")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.LeaseLog == nil {
		return errors.NotValidf("nil LeaseLog")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewTarget == nil {
		return errors.NotValidf("nil NewTarget")
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

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	st := statePool.SystemState()

	notifyTarget := config.NewTarget(st, raftlease.NewTargetLogger(config.LeaseLog, config.Logger))
	w, err := config.NewWorker(Config{
		Raft:                 r,
		Hub:                  hub,
		Logger:               config.Logger,
		Topic:                config.RequestTopic,
		Target:               notifyTarget,
		PrometheusRegisterer: config.PrometheusRegisterer,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// Manifold builds a dependency.Manifold for running a raftforwarder
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.RaftName,
			config.StateName,
			config.CentralHubName,
		},
		Start: config.start,
	}
}

// NewTarget is a shim to construct a raftlease.NotifyTarget for testability.
func NewTarget(st *state.State, logger raftleasestore.Logger) raftlease.NotifyTarget {
	return st.LeaseNotifyTarget(logger)
}
