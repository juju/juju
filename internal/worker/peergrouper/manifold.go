// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	coremodel "github.com/juju/juju/core/model"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
)

// ManifoldConfig holds the information necessary to run a peergrouper
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName          string
	ClockName          string
	StateName          string
	DomainServicesName string
	Hub                Hub

	PrometheusRegisterer prometheus.Registerer
	NewWorker            func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
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
	return nil
}

// Manifold returns a dependency.Manifold that will run a peergrouper.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ClockName,
			config.StateName,
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := getter.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.ControllerDomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigService := domainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	_, st, err := stTracker.Use()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}

	mongoSession := st.MongoSession()
	agentConfig := agent.CurrentConfig()

	ctrlModel, err := domainServices.Model().ControllerModel(ctx)
	if err != nil {
		_ = stTracker.Done()
		return nil, internalerrors.Errorf(
			"getting controller model to determine high availability support: %w", err,
		)
	}
	supportsHA := ctrlModel.ModelType != coremodel.CAAS

	w, err := config.NewWorker(Config{
		State:                   StateShim{State: st},
		ControllerConfigService: controllerConfigService,
		MongoSession:            MongoSessionShim{mongoSession},
		APIHostPortsSetter:      &CachingAPIHostPortsSetter{APIHostPortsSetter: st},
		Clock:                   clock,
		Hub:                     config.Hub,
		MongoPort:               controllerConfig.StatePort(),
		APIPort:                 controllerConfig.APIPort(),
		ControllerAPIPort:       controllerConfig.ControllerAPIPort(),
		SupportsHA:              supportsHA,
		PrometheusRegisterer:    config.PrometheusRegisterer,
		// On machine models, the controller id is the same as the machine/agent id.
		// TODO(wallyworld) - revisit when we add HA to k8s.
		ControllerId: agentConfig.Tag().Id,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}
