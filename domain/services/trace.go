// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/domain/tracing/state"
)

// TraceServices provides access to the services required by tracing.
type TraceServices struct {
	serviceFactoryBase
}

// NewTraceServices returns a new set of services for tracing.
func NewTraceServices(
	controllerDB changestream.WatchableDBFactory,
	logger logger.Logger,
) *TraceServices {
	return &TraceServices{
		serviceFactoryBase: serviceFactoryBase{
			controllerDB: controllerDB,
			logger:       logger,
		},
	}
}

// Tracing returns the tracing service which provides access to tracing
// configuration for charms and workloads.
func (s *TraceServices) Tracing() *service.WatchableService {
	return service.NewWatchableService(
		state.NewState(
			changestream.NewTxnRunnerFactory(s.controllerDB),
		),
		s.controllerWatcherFactory("tracing"),
	)
}
