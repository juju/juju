// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	controllernodestate "github.com/juju/juju/domain/controllernode/state"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	externalcontrollerstate "github.com/juju/juju/domain/externalcontroller/state"
	modelmanagerservice "github.com/juju/juju/domain/modelmanager/service"
	modelmanagerstate "github.com/juju/juju/domain/modelmanager/state"
)

// Logger defines the logging interface used by the services.
type Logger interface {
	Debugf(string, ...interface{})
	Child(string) Logger
}

// Registry provides access to the services required by the apiserver.
type Registry struct {
	controllerDB func() (changestream.WatchableDB, error)
	deleterDB    database.DBDeleter
	logger       Logger
}

// NewRegistry returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewRegistry(controllerDB func() (changestream.WatchableDB, error), logger Logger) *Registry {
	return &Registry{
		controllerDB: controllerDB,
		logger:       logger,
	}
}

// ControllerConfig returns the controller configuration service.
func (s *Registry) ControllerConfig() ControllerConfig {
	return controllerconfigservice.NewService(
		controllerconfigstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("controllerconfig"),
		),
	)
}

// ControllerNode returns the controller node service.
func (s *Registry) ControllerNode() ControllerNode {
	return controllernodeservice.NewService(
		controllernodestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// ModelManager returns the model manager service.
func (s *Registry) ModelManager() ModelManager {
	return modelmanagerservice.NewService(
		modelmanagerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.deleterDB,
	)
}

// ExternalController returns the external controller service.
func (s *Registry) ExternalController() ExternalController {
	return externalcontrollerservice.NewService(
		externalcontrollerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("externalcontroller"),
		),
	)
}
