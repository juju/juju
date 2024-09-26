// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package perf

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig holds the information necessary to run a performance test
// plan.
type ManifoldConfig struct {
	AgentName          string
	ServiceFactoryName string

	Clock                clock.Clock
	Logger               logger.Logger
	PrometheusRegisterer prometheus.Registerer
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an HTTP server
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ServiceFactoryName,
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
		return nil, err
	}

	currentModelUUID := model.UUID(agent.CurrentConfig().Model().Id())

	var serviceFactory servicefactory.ServiceFactory
	if err := getter.Get(config.ServiceFactoryName, &serviceFactory); err != nil {
		return nil, errors.Trace(err)
	}

	metricsCollector := NewMetricsCollector()
	if err := config.PrometheusRegisterer.Register(metricsCollector); err != nil {
		return nil, err
	}
	w, err := newPerfWorker(currentModelUUID, serviceFactory, config.Clock, config.Logger, metricsCollector)
	if err != nil {
		config.PrometheusRegisterer.Unregister(metricsCollector)
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() {
		config.PrometheusRegisterer.Unregister(metricsCollector)
	}), nil
}
