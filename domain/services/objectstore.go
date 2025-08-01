// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	controllernodestate "github.com/juju/juju/domain/controllernode/state"
	modelservice "github.com/juju/juju/domain/model/service"
	statemodel "github.com/juju/juju/domain/model/state/model"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
)

// ObjectStoreServices provides access to the services required by the
// apiserver.
type ObjectStoreServices struct {
	modelServiceFactoryBase
}

// NewObjectStoreServices returns a new set of services for the usage of the
// object store.
func NewObjectStoreServices(
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	logger logger.Logger,
) *ObjectStoreServices {
	return &ObjectStoreServices{
		modelServiceFactoryBase: modelServiceFactoryBase{
			serviceFactoryBase: serviceFactoryBase{
				controllerDB: controllerDB,
				logger:       logger,
			},
			modelDB: modelDB,
		},
	}
}

// ControllerConfig returns the controller configuration service.
func (s *ObjectStoreServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return controllerconfigservice.NewWatchableService(
		controllerconfigstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("controllerconfig"),
	)
}

// ControllerNode returns the controller node service.
func (s *ObjectStoreServices) ControllerNode() *controllernodeservice.WatchableService {
	return controllernodeservice.NewWatchableService(
		controllernodestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("controllernode"),
		s.logger.Child("controllernode"),
	)
}

// AgentObjectStore returns the object store service.
func (s *ObjectStoreServices) AgentObjectStore() *objectstoreservice.WatchableDrainingService {
	return objectstoreservice.NewWatchableDrainingService(
		objectstorestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("objectstore"),
	)
}

// ObjectStore returns the model's object store service.
func (s *ObjectStoreServices) ObjectStore() *objectstoreservice.WatchableService {
	return objectstoreservice.NewWatchableService(
		objectstorestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.modelWatcherFactory("objectstore"),
	)
}

// Model returns the provider model service.
func (s *ObjectStoreServices) Model() *modelservice.ObjectStoreService {
	return modelservice.NewObjectStoreService(
		statemodel.NewState(
			changestream.NewTxnRunnerFactory(s.modelDB),
			s.logger.Child("modelinfo"),
		),
		s.modelWatcherFactory("model"),
	)
}
