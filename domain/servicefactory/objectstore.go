// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
)

// ObjectStoreServices provides access to the services required by the
// apiserver.
type ObjectStoreServices struct {
	logger       logger.Logger
	controllerDB changestream.WatchableDBFactory
	modelDB      changestream.WatchableDBFactory
}

// NewObjectStoreServices returns a new set of services for the usage of the
// object store.
func NewObjectStoreServices(
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	logger logger.Logger,
) *ObjectStoreServices {
	return &ObjectStoreServices{
		logger:       logger,
		controllerDB: controllerDB,
		modelDB:      modelDB,
	}
}

// ControllerConfig returns the controller configuration service.
func (s *ObjectStoreServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return controllerconfigservice.NewWatchableService(
		controllerconfigstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("controllerconfig"),
		),
	)
}

// AgentObjectStore returns the object store service.
func (s *ObjectStoreServices) AgentObjectStore() *objectstoreservice.WatchableService {
	return objectstoreservice.NewWatchableService(
		objectstorestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(
			s.controllerDB,
			s.logger.Child("objectstore"),
		),
	)
}

// ObjectStore returns the model's object store service.
func (s *ObjectStoreServices) ObjectStore() *objectstoreservice.WatchableService {
	return objectstoreservice.NewWatchableService(
		objectstorestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		domain.NewWatcherFactory(
			s.modelDB,
			s.logger.Child("objectstore"),
		),
	)
}
