// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseservice

import (
	"io"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/core/raftlease"
	raftleasestore "github.com/juju/juju/state/raftlease"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// Logger specifies the interface we use from loggo.Logger.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
}

// State defines the methods used from state.
type State interface {
	LeaseNotifyTarget(raftleasestore.Logger) raftlease.NotifyTarget
}

// ManifoldConfig holds the information necessary to run an apiserver-based
// lease consumer worker in a dependency.Engine.
type ManifoldConfig struct {
	AuthenticatorName string
	MuxName           string
	RaftName          string
	StateName         string

	NewWorker func(Config) (worker.Worker, error)
	NewTarget func(State, raftleasestore.Logger) raftlease.NotifyTarget

	// Path is the path of the lease HTTP endpoint.
	Path string

	LeaseLog             io.Writer
	Logger               Logger
	Clock                clock.Clock
	PrometheusRegisterer prometheus.Registerer

	GetState func(workerstate.StateTracker) (State, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AuthenticatorName == "" {
		return errors.NotValidf("empty AuthenticatorName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.RaftName == "" {
		return errors.NotValidf("empty RaftName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewTarget == nil {
		return errors.NotValidf("nil NewTarget")
	}
	if config.Path == "" {
		return errors.NotValidf("empty Path")
	}
	if config.LeaseLog == nil {
		return errors.NotValidf("nil LeaseLog")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.GetState == nil {
		return errors.NotValidf("nil GetState")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an apiserver-based
// raft transport worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AuthenticatorName,
			config.MuxName,
			config.RaftName,
			config.StateName,
		},
		Start: config.start,
	}
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var r RaftApplier
	if err := context.Get(config.RaftName, &r); err != nil {
		return nil, errors.Trace(err)
	}

	var authenticator httpcontext.Authenticator
	if err := context.Get(config.AuthenticatorName, &authenticator); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	st, err := config.GetState(stTracker)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := context.Get(config.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	notifyTarget := config.NewTarget(st, raftlease.NewTargetLogger(config.LeaseLog, config.Logger))
	w, err := config.NewWorker(Config{
		Authenticator:        authenticator,
		Mux:                  mux,
		Path:                 config.Path,
		Raft:                 r,
		Target:               notifyTarget,
		Logger:               config.Logger,
		Clock:                config.Clock,
		PrometheusRegisterer: config.PrometheusRegisterer,
	})

	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// NewTarget is a shim to construct a raftlease.NotifyTarget for testability.
func NewTarget(st State, logger raftleasestore.Logger) raftlease.NotifyTarget {
	return st.LeaseNotifyTarget(logger)
}

func GetState(tracker workerstate.StateTracker) (State, error) {
	statePool, err := tracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	st := statePool.SystemState()
	return st, nil
}
