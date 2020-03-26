// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"crypto/tls"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
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
	CertGetterName string
	StateName      string
	Clock          clock.Clock
	MuxName        string
	NewWorker      func(Config) (worker.Worker, error)
	NewModelWorker NewModelWorkerFunc
	Logger         Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.CertGetterName == "" {
		return errors.NotValidf("empty CertGetterName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("emtpy MuxName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewModelWorker == nil {
		return errors.NotValidf("nil NewModelWorker")
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
			config.CertGetterName,
			config.MuxName,
			config.StateName,
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

	var certWatcher func() *tls.Certificate
	if err := context.Get(config.CertGetterName, &certWatcher); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := context.Get(config.MuxName, &mux); err != nil {
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

	w, err := config.NewWorker(Config{
		CertGetter:     certWatcher,
		Clock:          config.Clock,
		Logger:         config.Logger,
		MachineID:      machineID,
		ModelWatcher:   statePool.SystemState(),
		Mux:            mux,
		Controller:     StatePoolController{statePool},
		NewModelWorker: config.NewModelWorker,
		ErrorDelay:     jworker.RestartDelay,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { stTracker.Done() }), nil
}
