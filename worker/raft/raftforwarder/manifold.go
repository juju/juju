// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder

import (
	"io"
	"path/filepath"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

const (
	// maxLogs is the maximum number of backup lease log files to keep.
	maxLogs = 10

	// maxLogSizeMB is the maximum size of the lease log file on disk
	// in megabytes.
	maxLogSizeMB = 30
)

// ManifoldConfig holds the resources needed to start a raft forwarder
// worker in a dependency engine.
type ManifoldConfig struct {
	AgentName      string
	RaftName       string
	StateName      string
	CentralHubName string

	RequestTopic         string
	PrometheusRegisterer prometheus.Registerer
	Logger               Logger
	NewWorker            func(Config) (worker.Worker, error)
	NewTarget            func(*state.State, io.Writer, Logger) raftlease.NotifyTarget
}

// Validate checks that the config has all the required values.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
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

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
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

	logPath := filepath.Join(agent.CurrentConfig().LogDir(), "lease.log")
	if err := paths.PrimeLogFile(logPath); err != nil {
		// This isn't a fatal error, so log and continue if priming
		// fails.
		config.Logger.Warningf(
			"unable to prime log file %q (proceeding anyway): %s",
			logPath,
			err.Error(),
		)
	}

	notifyTarget := config.NewTarget(st, makeLogger(logPath), config.Logger)
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
			config.AgentName,
			config.RaftName,
			config.StateName,
			config.CentralHubName,
		},
		Start: config.start,
	}
}

// NewTarget is a shim to construct a raftlease.NotifyTarget for testability.
func NewTarget(st *state.State, logFile io.Writer, errorLog Logger) raftlease.NotifyTarget {
	return st.LeaseNotifyTarget(logFile, errorLog)
}

func makeLogger(path string) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxLogSizeMB,
		MaxBackups: maxLogs,
		Compress:   true,
	}
}
