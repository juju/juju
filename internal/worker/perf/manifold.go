// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package perf

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig holds the information necessary to run a performance test
// plan.
type ManifoldConfig struct {
	AgentName          string
	DomainServicesName string

	Clock  clock.Clock
	Logger logger.Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an HTTP server
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
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
		return nil, err
	}

	currentModelUUID := model.UUID(agent.CurrentConfig().Model().Id())

	var domainServices services.DomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, err
	}

	return newPerfWorker(
		currentModelUUID,
		domainServices,
		config.Clock,
		config.Logger,
	)
}
