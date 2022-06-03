// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pki"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
	"github.com/juju/juju/worker/syslogger"
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// ManifoldConfig holds the information necessary to run a model worker manager
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName      string
	AuthorityName  string
	StateName      string
	MuxName        string
	SyslogName     string
	Clock          clock.Clock
	NewWorker      func(Config) (worker.Worker, error)
	NewModelWorker NewModelWorkerFunc
	ModelMetrics   ModelMetrics
	Logger         Logger
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
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.SyslogName == "" {
		return errors.NotValidf("empty SyslogName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewModelWorker == nil {
		return errors.NotValidf("nil NewModelWorker")
	}
	if config.ModelMetrics == nil {
		return errors.NotValidf("nil ModelMetrics")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a model worker manager.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.AuthorityName,
			config.MuxName,
			config.StateName,
			config.SyslogName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var authority pki.Authority
	if err := context.Get(config.AuthorityName, &authority); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := context.Get(config.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	var sysLogger syslogger.SysLogger
	if err := context.Get(config.SyslogName, &sysLogger); err != nil {
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

	machineID := agent.CurrentConfig().Tag().Id()

	systemState, err := statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := config.NewWorker(Config{
		Authority:    authority,
		Clock:        config.Clock,
		Logger:       config.Logger,
		MachineID:    machineID,
		ModelWatcher: systemState,
		ModelMetrics: config.ModelMetrics,
		Mux:          mux,
		Controller: StatePoolController{
			StatePool: statePool,
			SysLogger: sysLogger,
		},
		NewModelWorker: config.NewModelWorker,
		ErrorDelay:     jworker.RestartDelay,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}
