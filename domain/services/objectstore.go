// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	controllerservice "github.com/juju/juju/domain/controller/service"
	controllerstate "github.com/juju/juju/domain/controller/state"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	modelobjectstoreservice "github.com/juju/juju/domain/model/service/objectstore"
	statemodel "github.com/juju/juju/domain/model/state/model"
	networkservice "github.com/juju/juju/domain/network/service"
	networkstate "github.com/juju/juju/domain/network/state"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
)

// ObjectStoreServices provides access to the services required by the
// apiserver.
type ObjectStoreServices struct {
	modelServiceFactoryBase

	clock          clock.Clock
	controllerUUID string
}

// NewObjectStoreServices returns a new set of services for the usage of the
// object store.
func NewObjectStoreServices(
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	controllerUUID string,
	clock clock.Clock,
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
		clock:          clock,
		controllerUUID: controllerUUID,
	}
}

// Controller returns the controller service.
func (s *ObjectStoreServices) Controller() *controllerservice.Service {
	return controllerservice.NewService(
		controllerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// ControllerConfig returns the controller configuration service.
func (s *ObjectStoreServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return controllerconfigservice.NewWatchableService(
		controllerconfigstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("controllerconfig"),
	)
}

// AgentObjectStore returns the object store service.
func (s *ObjectStoreServices) AgentObjectStore() *objectstoreservice.WatchableDrainingService {
	return objectstoreservice.NewWatchableDrainingService(
		objectstorestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), s.clock),
		s.controllerWatcherFactory("objectstore"),
		s.controllerUUID,
	)
}

// ObjectStore returns the model's object store service.
func (s *ObjectStoreServices) ObjectStore() *objectstoreservice.WatchableService {
	return objectstoreservice.NewWatchableService(
		objectstorestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), s.clock),
		s.modelWatcherFactory("objectstore"),
	)
}

// Model returns the provider model service.
func (s *ObjectStoreServices) Model() *modelobjectstoreservice.ObjectStoreService {
	return modelobjectstoreservice.NewObjectStoreService(
		statemodel.NewState(
			changestream.NewTxnRunnerFactory(s.modelDB),
			s.logger.Child("modelinfo"),
		),
		s.modelWatcherFactory("model"),
	)
}

// Network returns the controller model network service used for direct
// controller object-store traffic.
func (s *ObjectStoreServices) Network() *networkservice.WatchableService {
	return networkservice.NewWatchableService(
		networkstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), s.logger.Child("network")),
		func(context.Context) (networkservice.ProviderWithNetworking, error) {
			return nil, coreerrors.NotSupported
		},
		func(context.Context) (networkservice.ProviderWithZones, error) {
			return nil, coreerrors.NotSupported
		},
		s.modelWatcherFactory("network"), s.logger.Child("network"),
	)
}
