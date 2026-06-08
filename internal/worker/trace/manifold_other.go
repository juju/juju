//go:build !dqlite

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

// This file only exists to provide a noop implementation of the trace manifold
// for dqlite builds. This should be removed once the jujud and juju-agentd are
// full separated and the trace worker is only used in the right locations.

// GetTracingServiceFunc returns the controller tracing service from the
// dependency getter.
type GetTracingServiceFunc func(getter dependency.Getter, name string) (TracingService, error)

// ControllerManifoldConfig defines the configuration for the controller
// trace manifold.
type ControllerManifoldConfig struct {
	AgentName          string
	DomainServicesName string
	ChangeStreamName   string
	Clock              clock.Clock
	Logger             logger.Logger
	GetTracingService  GetTracingServiceFunc
	NewTracerWorker    TracerWorkerFunc
}

// Validate validates the controller manifold configuration.
func (cfg ControllerManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.GetTracingService == nil {
		return errors.NotValidf("nil GetTracingService")
	}
	if cfg.NewTracerWorker == nil {
		return errors.NotValidf("nil NewTracerWorker")
	}
	return nil
}

// ControllerManifold returns a dependency manifold that runs the controller
// trace worker with workload tracing hot-reload.
func ControllerManifold(config ControllerManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ChangeStreamName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}
			return NewNoopWorker(), nil
		},
	}
}

// TracingService is the interface that defines the methods required from the
// tracing service.
type TracingService any

// GetTracingService returns the controller tracing service from the
// dependency getter.
func GetTracingService(getter dependency.Getter, name string) (TracingService, error) {
	var controllerServices services.ControllerDomainServices
	if err := getter.Get(name, &controllerServices); err != nil {
		return nil, err
	}
	return controllerServices.Tracing(), nil
}
