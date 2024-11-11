// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"net/url"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	agentprovisionerservice "github.com/juju/juju/domain/agentprovisioner/service"
	agentprovisionerstate "github.com/juju/juju/domain/agentprovisioner/state"
	annotationService "github.com/juju/juju/domain/annotation/service"
	annotationState "github.com/juju/juju/domain/annotation/state"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	blockcommandservice "github.com/juju/juju/domain/blockcommand/service"
	blockcommandstate "github.com/juju/juju/domain/blockcommand/state"
	blockdeviceservice "github.com/juju/juju/domain/blockdevice/service"
	blockdevicestate "github.com/juju/juju/domain/blockdevice/state"
	cloudimagemetadataservice "github.com/juju/juju/domain/cloudimagemetadata/service"
	cloudimagemetadatastate "github.com/juju/juju/domain/cloudimagemetadata/state"
	keymanagerservice "github.com/juju/juju/domain/keymanager/service"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	keyupdaterservice "github.com/juju/juju/domain/keyupdater/service"
	keyupdaterstate "github.com/juju/juju/domain/keyupdater/state"
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
	modelmigrationservice "github.com/juju/juju/domain/modelmigration/service"
	modelmigrationstate "github.com/juju/juju/domain/modelmigration/state"
	networkservice "github.com/juju/juju/domain/network/service"
	networkstate "github.com/juju/juju/domain/network/state"
	portservice "github.com/juju/juju/domain/port/service"
	portstate "github.com/juju/juju/domain/port/state"
	proxy "github.com/juju/juju/domain/proxy/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretstate "github.com/juju/juju/domain/secret/state"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	storageservice "github.com/juju/juju/domain/storage/service"
	storagestate "github.com/juju/juju/domain/storage/state"
	stubservice "github.com/juju/juju/domain/stub"
	unitstateservice "github.com/juju/juju/domain/unitstate/service"
	unitstatestate "github.com/juju/juju/domain/unitstate/state"
	"github.com/juju/juju/environs/config"
)

// PublicKeyImporter describes a service that is capable of fetching and
// providing public keys for a subject from a set of well known sources that
// don't need to be understood by this service.
type PublicKeyImporter interface {
	// FetchPublicKeysForSubject is responsible for gathering all of the
	// public keys available for a specified subject.
	// The following errors can be expected:
	// - [importererrors.NoResolver] when there is import resolver the subject
	// schema.
	// - [importerrors.SubjectNotFound] when the resolver has reported that no
	// subject exists.
	FetchPublicKeysForSubject(context.Context, *url.URL) ([]string, error)
}

// ModelFactory provides access to the services required by the apiserver.
type ModelFactory struct {
	clock             clock.Clock
	logger            logger.Logger
	controllerDB      changestream.WatchableDBFactory
	modelUUID         model.UUID
	modelDB           changestream.WatchableDBFactory
	providerFactory   providertracker.ProviderFactory
	objectstore       objectstore.ModelObjectStoreGetter
	storageRegistry   corestorage.ModelStorageRegistryGetter
	publicKeyImporter PublicKeyImporter
	leaseManager      lease.ModelLeaseManagerGetter
}

// NewModelFactory returns a new registry which uses the provided modelDB
// function to obtain a model database.
func NewModelFactory(
	modelUUID model.UUID,
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	providerFactory providertracker.ProviderFactory,
	objectStore objectstore.ModelObjectStoreGetter,
	storageRegistry corestorage.ModelStorageRegistryGetter,
	publicKeyImporter PublicKeyImporter,
	leaseManager lease.ModelLeaseManagerGetter,
	clock clock.Clock,
	logger logger.Logger,
) *ModelFactory {
	return &ModelFactory{
		clock:             clock,
		logger:            logger,
		controllerDB:      controllerDB,
		modelUUID:         modelUUID,
		modelDB:           modelDB,
		providerFactory:   providerFactory,
		objectstore:       objectStore,
		storageRegistry:   storageRegistry,
		publicKeyImporter: publicKeyImporter,
		leaseManager:      leaseManager,
	}
}

// AgentProvisioner returns the agent provisioner service.
func (s *ModelFactory) AgentProvisioner() *agentprovisionerservice.Service {
	return agentprovisionerservice.NewService(
		agentprovisionerstate.NewState(
			changestream.NewTxnRunnerFactory(s.modelDB),
		),
		providertracker.ProviderRunner[agentprovisionerservice.Provider](s.providerFactory, s.modelUUID.String()),
	)
}

// Config returns the model's configuration service.
func (s *ModelFactory) Config() *modelconfigservice.WatchableService {
	defaultsProvider := modeldefaultsservice.NewService(
		modeldefaultsservice.ProviderModelConfigGetter(),
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

// Machine returns the model's machine service.
func (s *ModelFactory) Machine() *machineservice.WatchableService {
	return machineservice.NewWatchableService(
		machinestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), s.logger.Child("machine")),
		domain.NewWatcherFactory(
			s.modelDB,
			s.logger.Child("machine"),
		),
		providertracker.ProviderRunner[machineservice.Provider](s.providerFactory, s.modelUUID.String()),
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
func (s *ModelFactory) Application() *applicationservice.WatchableService {
	return applicationservice.NewWatchableService(
		applicationstate.NewApplicationState(changestream.NewTxnRunnerFactory(s.modelDB),
			s.logger.Child("application"),
		),
		secretstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB),
			s.logger.Child("application"),
		),
		applicationstate.NewCharmState(changestream.NewTxnRunnerFactory(s.modelDB)),
		domain.NewWatcherFactory(s.modelDB, s.logger.Child("application")),
		s.modelUUID,
		modelagentstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		providertracker.ProviderRunner[applicationservice.Provider](s.providerFactory, s.modelUUID.String()),
		s.storageRegistry,
		s.logger.Child("application"),
	)
}

// KeyManager  returns the model's user public ssh key manager. Use this service
// when wanting to modify a user's public ssh keys within a model.
func (s *ModelFactory) KeyManager() *keymanagerservice.Service {
	return keymanagerservice.NewService(
		s.modelUUID,
		keymanagerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// KeyManagerWithImporter returns the model's user public ssh key manager with
// the ability to import ssh public keys from external sources. Use this service
// when wanting to modify a user's public ssh keys within a model.
func (s *ModelFactory) KeyManagerWithImporter() *keymanagerservice.ImporterService {
	return keymanagerservice.NewImporterService(
		s.modelUUID,
		s.publicKeyImporter,
		keymanagerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// KeyUpdater returns the model's key updater service. Use this service when
// wanting to retrieve the authorised ssh public keys for a model.
func (s *ModelFactory) KeyUpdater() *keyupdaterservice.WatchableService {
	controllerState := keyupdaterstate.NewControllerState(
		changestream.NewTxnRunnerFactory(s.controllerDB),
	)
	return keyupdaterservice.NewWatchableService(
		keyupdaterservice.NewControllerKeyService(
			controllerState,
		),
		controllerState,
		keyupdaterstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		domain.NewWatcherFactory(s.controllerDB, s.logger.Child("keyupdater")),
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
func (s *ModelFactory) Storage() *storageservice.Service {
	return storageservice.NewService(
		storagestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.logger.Child("storage"),
		s.storageRegistry,
	)
}

// Secret returns the model's secret service.
func (s *ModelFactory) Secret(params secretservice.SecretServiceParams) *secretservice.WatchableService {
	logger := s.logger.Child("secret")
	return secretservice.NewWatchableService(
		secretstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), logger.Child("state")),
		secretbackendstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), logger.Child("secretbackendstate")),
		logger.Child("service"),
		domain.NewWatcherFactory(s.modelDB, logger.Child("watcherfactory")),
		params,
	)
}

// ModelMigration returns the model's migration service for supporting migration
// operations.
func (s *ModelFactory) ModelMigration() *modelmigrationservice.Service {
	return modelmigrationservice.NewService(
		providertracker.ProviderRunner[modelmigrationservice.InstanceProvider](s.providerFactory, s.modelUUID.String()),
		providertracker.ProviderRunner[modelmigrationservice.ResourceProvider](s.providerFactory, s.modelUUID.String()),
		modelmigrationstate.New(changestream.NewTxnRunnerFactory(s.modelDB)),
	)
}

// ModelSecretBackend returns the model secret backend service.
func (s *ModelFactory) ModelSecretBackend() *secretbackendservice.ModelSecretBackendService {
	logger := s.logger.Child("modelsecretbackend")
	state := secretbackendstate.NewState(
		changestream.NewTxnRunnerFactory(s.controllerDB),
		logger.Child("state"),
	)
	return secretbackendservice.NewModelSecretBackendService(
		s.modelUUID, state,
	)
}

// Agent returns the model's agent service.
func (s *ModelFactory) Agent() *modelagentservice.ModelService {
	return modelagentservice.NewModelService(
		modelagentstate.NewModelState(changestream.NewTxnRunnerFactory(s.modelDB)),
		modelagentstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		domain.NewWatcherFactory(s.modelDB, s.logger.Child("modelagent.watcherfactory")),
	)
}

// ModelInfo returns the model info service.
func (s *ModelFactory) ModelInfo() *modelservice.ModelService {
	return modelservice.NewModelService(
		s.modelUUID,
		modelstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		modelstate.NewModelState(changestream.NewTxnRunnerFactory(s.modelDB), s.logger.Child("modelinfo")),
	)
}

// Proxy returns the proxy service.
func (s *ModelFactory) Proxy() *proxy.Service {
	return proxy.NewService(
		providertracker.ProviderRunner[proxy.Provider](s.providerFactory, s.modelUUID.String()),
	)
}

// UnitState returns the service for persisting and retrieving remote unit
// state. This is used to reconcile with local state to determine which
// hooks to run, and is saved upon hook completion.
func (s *ModelFactory) UnitState() *unitstateservice.Service {
	return unitstateservice.NewService(
		unitstatestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
	)
}

// CloudImageMetadata returns the service for persisting and retrieving cloud image metadata for the current model.
func (s *ModelFactory) CloudImageMetadata() *cloudimagemetadataservice.Service {
	return cloudimagemetadataservice.NewService(
		cloudimagemetadatastate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), s.clock, s.logger.Child("cloudimagemetadata")),
	)
}

// Port returns the service for managing opened port ranges for units.
func (s *ModelFactory) Port() *portservice.WatchableService {
	logger := s.logger.Child("port")
	return portservice.NewWatchableService(
		portstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		domain.NewWatcherFactory(s.modelDB, logger.Child("watcherfactory")),
		logger.Child("service"),
	)
}

// Stub returns the stub service. A special service which collects temporary
// methods required to wire together domains which are not completely implemented
// or wired up.
//
// Deprecated: Stub service contains only temporary methods and should be removed
// as soon as possible.
func (s *ModelFactory) Stub() *stubservice.StubService {
	return stubservice.NewStubService(
		changestream.NewTxnRunnerFactory(s.modelDB),
	)
}

// BlockCommand returns the service for blocking commands.
func (s *ModelFactory) BlockCommand() *blockcommandservice.Service {
	return blockcommandservice.NewService(
		blockcommandstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.logger.Child("blockcommand"),
	)
}
