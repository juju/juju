// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	modelservice "github.com/juju/juju/domain/model/service"
	modelstate "github.com/juju/juju/domain/model/state"
)

// LogSinkServices provides access to the services required by the
// apiserver.
type LogSinkServices struct {
	modelServiceFactoryBase
}

// NewLogSinkServices returns a new set of services for the usage of the
// object store.
func NewLogSinkServices(
	controllerDB changestream.WatchableDBFactory,
	logger logger.Logger,
) *LogSinkServices {
	return &LogSinkServices{
		modelServiceFactoryBase: modelServiceFactoryBase{
			serviceFactoryBase: serviceFactoryBase{
				controllerDB: controllerDB,
				logger:       logger,
			},
		},
	}
}

// ControllerConfig returns the controller configuration service.
func (s *LogSinkServices) ControllerConfig() *controllerconfigservice.WatchableService {
	logger := s.loggerFor("controllerconfig")

	return controllerconfigservice.NewWatchableService(
		controllerconfigstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory(logger),
	)
}

// Model returns the provider model service.
func (s *LogSinkServices) Model() *modelservice.LogSinkService {
	return modelservice.NewLogSinkService(
		modelstate.NewState(
			changestream.NewTxnRunnerFactory(s.controllerDB),
		),
	)
}
