// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Criticalf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// ManifoldConfig holds the information necessary to run a peergrouper
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName          string
	ClockName          string
	ControllerPortName string
	StateName          string
	ChangeStreamName   string
	Hub                Hub
	Logger             Logger

	PrometheusRegisterer       prometheus.Registerer
	NewWorker                  func(Config) (worker.Worker, error)
	NewControllerConfigService func(getter changestream.WatchableDBGetter, logger Logger) ControllerConfigService
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.ControllerPortName == "" {
		return errors.NotValidf("empty ControllerPortName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.NewControllerConfigService == nil {
		return errors.NotValidf("nil NewControllerConfigService")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a peergrouper.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ClockName,
			config.ControllerPortName,
			config.StateName,
			config.ChangeStreamName,
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

	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	// Ensure that the controller-port worker is running.
	if err := context.Get(config.ControllerPortName, nil); err != nil {
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

	st, err := statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	mongoSession := st.MongoSession()
	agentConfig := agent.CurrentConfig()
	stateServingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		_ = stTracker.Done()
		return nil, errors.New("state serving info missing from agent config")
	}
	model, err := st.Model()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	supportsHA := model.Type() != state.ModelTypeCAAS

	var dbGetter changestream.WatchableDBGetter
	if err := context.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Annotate(err, "failed to get DB getter")
	}

	w, err := config.NewWorker(Config{
		State:                StateShim{st},
		MongoSession:         MongoSessionShim{mongoSession},
		APIHostPortsSetter:   &CachingAPIHostPortsSetter{APIHostPortsSetter: st},
		Clock:                clock,
		Hub:                  config.Hub,
		MongoPort:            stateServingInfo.StatePort,
		APIPort:              stateServingInfo.APIPort,
		ControllerAPIPort:    stateServingInfo.ControllerAPIPort,
		SupportsHA:           supportsHA,
		PrometheusRegisterer: config.PrometheusRegisterer,
		// On machine models, the controller id is the same as the machine/agent id.
		// TODO(wallyworld) - revisit when we add HA to k8s.
		ControllerId:      agentConfig.Tag().Id,
		CtrlConfigService: config.NewControllerConfigService(dbGetter, config.Logger),
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// NewControllerConfigService returns a new ControllerConfigService.
func NewControllerConfigService(dbGetter changestream.WatchableDBGetter, logger Logger) ControllerConfigService {
	return controllerconfigservice.NewService(
		controllerconfigstate.NewState(coredatabase.NewTxnRunnerFactoryForNamespace(
			dbGetter.GetWatchableDB,
			coredatabase.ControllerNS,
		)),
		domain.NewWatcherFactory(
			func() (changestream.WatchableDB, error) {
				return dbGetter.GetWatchableDB(coredatabase.ControllerNS)
			},
			logger,
		),
	)
}
