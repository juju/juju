// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	annotationService "github.com/juju/juju/domain/annotation/service"
	annotationState "github.com/juju/juju/domain/annotation/state"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	blockdeviceservice "github.com/juju/juju/domain/blockdevice/service"
	blockdevicestate "github.com/juju/juju/domain/blockdevice/state"
	machineservice "github.com/juju/juju/domain/machine/service"
	machinestate "github.com/juju/juju/domain/machine/state"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	modelconfigstate "github.com/juju/juju/domain/modelconfig/state"
	networkservice "github.com/juju/juju/domain/network/service"
	networkstate "github.com/juju/juju/domain/network/state"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
	storageservice "github.com/juju/juju/domain/storage/service"
	storagestate "github.com/juju/juju/domain/storage/state"
	unitservice "github.com/juju/juju/domain/unit/service"
	unitstate "github.com/juju/juju/domain/unit/state"
	"github.com/juju/juju/internal/storage"
)

// ModelFactory provides access to the services required by the apiserver.
type ModelFactory struct {
	logger  Logger
	modelDB changestream.WatchableDBFactory
}

// NewModelFactory returns a new registry which uses the provided modelDB
// function to obtain a model database.
func NewModelFactory(
	modelDB changestream.WatchableDBFactory,
	logger Logger,
) *ModelFactory {
	return &ModelFactory{
		logger:  logger,
		modelDB: modelDB,
	}
}

// Config returns the model's configuration service. A ModelDefaultsProvider
// needs to be supplied for the model config service. The provider can be
// obtained from the controller service factory model defaults service.
func (s *ModelFactory) Config(
	defaultsProvider modelconfigservice.ModelDefaultsProvider,
) *modelconfigservice.WatchableService {
	return modelconfigservice.NewWatchableService(
		defaultsProvider,
		modelconfigstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		domain.NewWatcherFactory(s.modelDB, s.logger.Child("modelconfig")),
	)
}

// ObjectStore returns the model's object store service.
func (s *ModelFactory) ObjectStore() *objectstoreservice.WatchableService {
	return objectstoreservice.NewWatchableService(
		objectstorestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		domain.NewWatcherFactory(
			s.modelDB,
			s.logger.Child("objectstore"),
		),
	)
}

// Machine returns the model's machine service.
func (s *ModelFactory) Machine() *machineservice.Service {
	return machineservice.NewService(
		machinestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), s.logger.Child("machine")),
	)
}

// BlockDevice returns the model's block device service.
func (s *ModelFactory) BlockDevice() *blockdeviceservice.WatchableService {
	return blockdeviceservice.NewWatchableService(
		blockdevicestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		domain.NewWatcherFactory(s.modelDB, s.logger.Child("blockdevice")),
		s.logger.Child("blockdevice"),
	)
}

// Application returns the model's application service.
func (s *ModelFactory) Application(registry storage.ProviderRegistry) *applicationservice.Service {
	return applicationservice.NewService(
		applicationstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB),
			s.logger.Child("application"),
		),
		s.logger.Child("application"),
		registry,
	)
}

// Unit returns the model's unit service.
func (s *ModelFactory) Unit() *unitservice.Service {
	return unitservice.NewService(
		unitstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB),
			s.logger.Child("unit"),
		),
	)
}

// Space returns the model's space service.
func (s *ModelFactory) Space() *networkservice.SpaceService {
	return networkservice.NewSpaceService(
		networkstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.logger.Child("space"),
	)
}

// Annotation returns the model's annotation service.
func (s *ModelFactory) Annotation() *annotationService.Service {
	return annotationService.NewService(
		annotationState.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
	)
}

// Storage returns the model's storage service.
func (s *ModelFactory) Storage(registry storage.ProviderRegistry) *storageservice.Service {
	return storageservice.NewService(
		storagestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.logger.Child("storage"),
		registry,
	)
}
