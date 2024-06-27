// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/domain"
	annotationService "github.com/juju/juju/domain/annotation/service"
	annotationState "github.com/juju/juju/domain/annotation/state"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	blockdeviceservice "github.com/juju/juju/domain/blockdevice/service"
	blockdevicestate "github.com/juju/juju/domain/blockdevice/state"
	machineservice "github.com/juju/juju/domain/machine/service"
	machinestate "github.com/juju/juju/domain/machine/state"
	modelservice "github.com/juju/juju/domain/model/service"
	modelstate "github.com/juju/juju/domain/model/state"
	modelagentservice "github.com/juju/juju/domain/modelagent/service"
	modelagentstate "github.com/juju/juju/domain/modelagent/state"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	modelconfigstate "github.com/juju/juju/domain/modelconfig/state"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	modeldefaultsstate "github.com/juju/juju/domain/modeldefaults/state"
	networkservice "github.com/juju/juju/domain/network/service"
	networkstate "github.com/juju/juju/domain/network/state"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretstate "github.com/juju/juju/domain/secret/state"
	storageservice "github.com/juju/juju/domain/storage/service"
	storagestate "github.com/juju/juju/domain/storage/state"
	unitservice "github.com/juju/juju/domain/unit/service"
	unitstate "github.com/juju/juju/domain/unit/state"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/storage"
)

// ModelFactory provides access to the services required by the apiserver.
type ModelFactory struct {
	logger          logger.Logger
	controllerDB    changestream.WatchableDBFactory
	modelUUID       model.UUID
	modelDB         changestream.WatchableDBFactory
	providerFactory providertracker.ProviderFactory
}

// NewModelFactory returns a new registry which uses the provided modelDB
// function to obtain a model database.
func NewModelFactory(
	modelUUID model.UUID,
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	providerFactory providertracker.ProviderFactory,
	logger logger.Logger,
) *ModelFactory {
	return &ModelFactory{
		logger:          logger,
		controllerDB:    controllerDB,
		modelUUID:       modelUUID,
		modelDB:         modelDB,
		providerFactory: providerFactory,
	}
}

// Config returns the model's configuration service.
func (s *ModelFactory) Config() *modelconfigservice.WatchableService {
	defaultsProvider := modeldefaultsservice.NewService(
		modeldefaultsstate.NewState(
			changestream.NewTxnRunnerFactory(s.controllerDB),
		)).ModelDefaultsProvider(s.modelUUID)

	return modelconfigservice.NewWatchableService(
		defaultsProvider,
		config.ModelValidator(),
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
func (s *ModelFactory) Machine() *machineservice.WatchableService {
	return machineservice.NewWatchableService(
		machinestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), s.logger.Child("machine")),
		domain.NewWatcherFactory(
			s.modelDB,
			s.logger.Child("machine"),
		),
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

// Network returns the model's network service.
func (s *ModelFactory) Network() *networkservice.WatchableService {
	return networkservice.NewWatchableService(
		networkstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), s.logger.Child("network.state")),
		providertracker.ProviderRunner[networkservice.Provider](s.providerFactory, s.modelUUID.String()),
		domain.NewWatcherFactory(s.modelDB, s.logger.Child("network.watcherfactory")),
		s.logger.Child("network.service"),
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

// Secret returns the model's secret service.
func (s *ModelFactory) Secret(adminConfigGetter secretservice.BackendAdminConfigGetter) *secretservice.WatchableService {
	logger := s.logger.Child("secret")
	return secretservice.NewWatchableService(
		secretstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), logger.Child("state")),
		logger.Child("service"),
		domain.NewWatcherFactory(s.modelDB, logger.Child("watcherfactory")),
		adminConfigGetter,
	)
}

// Agent returns the model's agent service.
func (s *ModelFactory) Agent() *modelagentservice.ModelService {
	return modelagentservice.NewModelService(
		modelagentstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.modelUUID,
	)
}

// ModelInfo returns the model info service.
func (s *ModelFactory) ModelInfo() *modelservice.ModelService {
	return modelservice.NewModelService(
		s.modelUUID,
		modelstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		modelstate.NewModelState(changestream.NewTxnRunnerFactory(s.modelDB)),
	)
}
