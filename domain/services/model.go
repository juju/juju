// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"net/url"
	"path/filepath"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	agentbinaryservice "github.com/juju/juju/domain/agentbinary/service"
	agentbinarystate "github.com/juju/juju/domain/agentbinary/state"
	agentpasswordservice "github.com/juju/juju/domain/agentpassword/service"
	agentpasswordstate "github.com/juju/juju/domain/agentpassword/state"
	agentprovisionerservice "github.com/juju/juju/domain/agentprovisioner/service"
	agentprovisionerstate "github.com/juju/juju/domain/agentprovisioner/state"
	annotationService "github.com/juju/juju/domain/annotation/service"
	annotationState "github.com/juju/juju/domain/annotation/state"
	charmstore "github.com/juju/juju/domain/application/charm/store"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	blockcommandservice "github.com/juju/juju/domain/blockcommand/service"
	blockcommandstate "github.com/juju/juju/domain/blockcommand/state"
	blockdeviceservice "github.com/juju/juju/domain/blockdevice/service"
	blockdevicestate "github.com/juju/juju/domain/blockdevice/state"
	cloudimagemetadataservice "github.com/juju/juju/domain/cloudimagemetadata/service"
	cloudimagemetadatastate "github.com/juju/juju/domain/cloudimagemetadata/state"
	containerimageresourcestoreservice "github.com/juju/juju/domain/containerimageresourcestore/service"
	containerimageresourcestorestate "github.com/juju/juju/domain/containerimageresourcestore/state"
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
	modelproviderservice "github.com/juju/juju/domain/modelprovider/service"
	modelproviderstate "github.com/juju/juju/domain/modelprovider/state"
	networkservice "github.com/juju/juju/domain/network/service"
	networkstate "github.com/juju/juju/domain/network/state"
	portservice "github.com/juju/juju/domain/port/service"
	portstate "github.com/juju/juju/domain/port/state"
	proxy "github.com/juju/juju/domain/proxy/service"
	relationservice "github.com/juju/juju/domain/relation/service"
	relationstate "github.com/juju/juju/domain/relation/state"
	removalservice "github.com/juju/juju/domain/removal/service"
	removalstatecontroller "github.com/juju/juju/domain/removal/state/controller"
	removalstatemodel "github.com/juju/juju/domain/removal/state/model"
	resolveservice "github.com/juju/juju/domain/resolve/service"
	resolveState "github.com/juju/juju/domain/resolve/state"
	resourceservice "github.com/juju/juju/domain/resource/service"
	resourcestate "github.com/juju/juju/domain/resource/state"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretstate "github.com/juju/juju/domain/secret/state"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	statusservice "github.com/juju/juju/domain/status/service"
	statusstate "github.com/juju/juju/domain/status/state"
	storageservice "github.com/juju/juju/domain/storage/service"
	storagestate "github.com/juju/juju/domain/storage/state"
	storageprovisioningservice "github.com/juju/juju/domain/storageprovisioning/service"
	storageprovisioningstate "github.com/juju/juju/domain/storageprovisioning/state"
	stubservice "github.com/juju/juju/domain/stub"
	unitstateservice "github.com/juju/juju/domain/unitstate/service"
	unitstatestate "github.com/juju/juju/domain/unitstate/state"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/resource/store"
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

// ModelServices provides access to the services required by the apiserver.
type ModelServices struct {
	modelServiceFactoryBase

	modelUUID              model.UUID
	providerFactory        providertracker.ProviderFactory
	modelObjectStoreGetter objectstore.ModelObjectStoreGetter
	storageRegistry        corestorage.ModelStorageRegistryGetter
	publicKeyImporter      PublicKeyImporter
	leaseManager           lease.ModelLeaseManagerGetter
	logDir                 string
	clock                  clock.Clock
}

// NewModelServices returns a new registry which uses the provided modelDB
// function to obtain a model database.
func NewModelServices(
	modelUUID model.UUID,
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	providerFactory providertracker.ProviderFactory,
	modelObjectStoreGetter objectstore.ModelObjectStoreGetter,
	storageRegistry corestorage.ModelStorageRegistryGetter,
	publicKeyImporter PublicKeyImporter,
	leaseManager lease.ModelLeaseManagerGetter,
	logDir string,
	clock clock.Clock,
	logger logger.Logger,
) *ModelServices {
	return &ModelServices{
		modelServiceFactoryBase: modelServiceFactoryBase{
			serviceFactoryBase: serviceFactoryBase{
				controllerDB: controllerDB,
				logger:       logger,
			},
			modelDB: modelDB,
		},
		modelUUID:              modelUUID,
		providerFactory:        providerFactory,
		modelObjectStoreGetter: modelObjectStoreGetter,
		storageRegistry:        storageRegistry,
		publicKeyImporter:      publicKeyImporter,
		leaseManager:           leaseManager,
		logDir:                 logDir,
		clock:                  clock,
	}
}

// AgentBinaryStore returns the model's [agentbinaryservice.AgentBinaryStore]
// for the current model.
func (s *ModelServices) AgentBinaryStore() *agentbinaryservice.AgentBinaryStore {
	return agentbinaryservice.NewAgentBinaryStore(
		agentbinarystate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.logger.Child("modelagentbinary"),
		s.modelObjectStoreGetter,
	)
}

// AgentBinary returns the model's [agentbinaryservice.AgentBinaryService].
func (s *ModelServices) AgentBinary() *agentbinaryservice.AgentBinaryService {
	return agentbinaryservice.NewAgentBinaryService(
		agentbinarystate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		agentbinarystate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		providertracker.ProviderRunner[agentbinaryservice.ProviderForAgentBinaryFinder](
			s.providerFactory, s.modelUUID.String(),
		),
		envtools.PreferredStreams, envtools.FindTools,
	)
}

// AgentProvisioner returns the agent provisioner service.
func (s *ModelServices) AgentProvisioner() *agentprovisionerservice.Service {
	return agentprovisionerservice.NewService(
		agentprovisionerstate.NewState(
			changestream.NewTxnRunnerFactory(s.modelDB),
		),
		providertracker.ProviderRunner[agentprovisionerservice.Provider](s.providerFactory, s.modelUUID.String()),
	)
}

// Config returns the model's configuration service.
func (s *ModelServices) Config() *modelconfigservice.WatchableService {
	defaultsProvider := modeldefaultsservice.NewService(
		modeldefaultsservice.ProviderModelConfigGetter(),
		modeldefaultsstate.NewState(
			changestream.NewTxnRunnerFactory(s.controllerDB),
		)).ModelDefaultsProvider(s.modelUUID)

	return modelconfigservice.NewWatchableService(
		defaultsProvider,
		config.ModelValidator(),
		modelconfigstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.modelWatcherFactory("modelconfig"),
	)
}

// Machine returns the model's machine service.
func (s *ModelServices) Machine() *machineservice.WatchableService {
	logger := s.logger.Child("machine")

	return machineservice.NewWatchableService(
		machinestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), s.clock, logger),
		s.modelWatcherFactory("machine"),
		providertracker.ProviderRunner[machineservice.Provider](s.providerFactory, s.modelUUID.String()),
		providertracker.ProviderRunner[machineservice.LXDProfileProvider](s.providerFactory, s.modelUUID.String()),
		domain.NewStatusHistory(logger, s.clock),
		s.clock,
		logger,
	)
}

// BlockDevice returns the model's block device service.
func (s *ModelServices) BlockDevice() *blockdeviceservice.WatchableService {
	return blockdeviceservice.NewWatchableService(
		blockdevicestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.modelWatcherFactory("blockdevice"),
		s.logger.Child("blockdevice"),
	)
}

// Application returns the model's application service.
func (s *ModelServices) Application() *applicationservice.WatchableService {
	logger := s.logger.Child("application")
	state := applicationstate.NewState(
		changestream.NewTxnRunnerFactory(s.modelDB), s.clock, logger,
	)

	storageValidator := applicationservice.NewStorageProviderValidator(
		s.storageRegistry, state,
	)

	return applicationservice.NewWatchableService(
		state,
		domain.NewLeaseService(s.leaseManager),
		s.modelWatcherFactory("application"),
		modelagentstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		providertracker.ProviderRunner[applicationservice.Provider](s.providerFactory, s.modelUUID.String()),
		providertracker.ProviderRunner[applicationservice.CAASProvider](s.providerFactory, s.modelUUID.String()),
		storageValidator,
		charmstore.NewCharmStore(s.modelObjectStoreGetter, logger.Child("charmstore")),
		domain.NewStatusHistory(logger, s.clock),
		s.clock,
		logger,
	)
}

// Status returns the application status service.
func (s *ModelServices) Status() *statusservice.LeadershipService {
	logger := s.logger.Child("status")
	return statusservice.NewLeadershipService(
		statusstate.NewModelState(changestream.NewTxnRunnerFactory(s.modelDB), s.clock, logger),
		statusstate.NewControllerState(changestream.NewTxnRunnerFactory(s.controllerDB), s.modelUUID),
		domain.NewLeaseService(s.leaseManager),
		s.modelUUID,
		domain.NewStatusHistory(logger, s.clock),
		func() (statusservice.StatusHistoryReader, error) {
			logsink := filepath.Join(s.logDir, "logsink.log")
			return domain.NewStatusHistoryReader(logsink, s.modelUUID)
		},
		s.clock,
		logger,
	)
}

// Resolve returns the resolve service.
func (s *ModelServices) Resolve() *resolveservice.WatchableService {
	return resolveservice.NewWatchableService(
		resolveState.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.modelWatcherFactory("resolve"),
	)
}

// KeyManager  returns the model's user public ssh key manager. Use this service
// when wanting to modify a user's public ssh keys within a model.
func (s *ModelServices) KeyManager() *keymanagerservice.Service {
	return keymanagerservice.NewService(
		s.modelUUID,
		keymanagerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// KeyManagerWithImporter returns the model's user public ssh key manager with
// the ability to import ssh public keys from external sources. Use this service
// when wanting to modify a user's public ssh keys within a model.
func (s *ModelServices) KeyManagerWithImporter() *keymanagerservice.ImporterService {
	return keymanagerservice.NewImporterService(
		s.modelUUID,
		s.publicKeyImporter,
		keymanagerstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// KeyUpdater returns the model's key updater service. Use this service when
// wanting to retrieve the authorised ssh public keys for a model.
func (s *ModelServices) KeyUpdater() *keyupdaterservice.WatchableService {
	controllerState := keyupdaterstate.NewControllerState(
		changestream.NewTxnRunnerFactory(s.controllerDB),
	)
	return keyupdaterservice.NewWatchableService(
		keyupdaterservice.NewControllerKeyService(controllerState),
		controllerState,
		keyupdaterstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.controllerWatcherFactory("keyupdater"),
	)
}

// Network returns the model's network service.
func (s *ModelServices) Network() *networkservice.WatchableService {
	log := s.logger.Child("network")

	return networkservice.NewWatchableService(
		networkstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), log),
		providertracker.ProviderRunner[networkservice.ProviderWithNetworking](s.providerFactory, s.modelUUID.String()),
		providertracker.ProviderRunner[networkservice.ProviderWithZones](s.providerFactory, s.modelUUID.String()),
		s.modelWatcherFactory("network"),
		log,
	)
}

// Annotation returns the model's annotation service.
func (s *ModelServices) Annotation() *annotationService.Service {
	return annotationService.NewService(
		annotationState.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
	)
}

// Storage returns the model's storage service.
func (s *ModelServices) Storage() *storageservice.Service {
	return storageservice.NewService(
		storagestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.logger.Child("storage"),
		s.storageRegistry,
	)
}

// StorageProvisioning returns the model's storage provisioning service.
func (s *ModelServices) StorageProvisioning() *storageprovisioningservice.Service {
	return storageprovisioningservice.NewService(
		storageprovisioningstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.modelWatcherFactory("storageprovisioning"),
	)
}

// Secret returns the model's secret service.
func (s *ModelServices) Secret() *secretservice.WatchableService {
	log := s.logger.Child("secret")
	return secretservice.NewWatchableService(
		secretstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB), log),
		secretbackendstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), log),
		domain.NewLeaseService(s.leaseManager),
		s.modelWatcherFactory("secret"),
		log,
	)
}

// ModelMigration returns the model's migration service for supporting migration
// operations.
func (s *ModelServices) ModelMigration() *modelmigrationservice.Service {
	return modelmigrationservice.NewService(
		providertracker.ProviderRunner[modelmigrationservice.InstanceProvider](s.providerFactory, s.modelUUID.String()),
		providertracker.ProviderRunner[modelmigrationservice.ResourceProvider](s.providerFactory, s.modelUUID.String()),
		modelmigrationstate.New(changestream.NewTxnRunnerFactory(s.modelDB)),
	)
}

// ModelSecretBackend returns the model secret backend service.
func (s *ModelServices) ModelSecretBackend() *secretbackendservice.ModelSecretBackendService {
	state := secretbackendstate.NewState(
		changestream.NewTxnRunnerFactory(s.controllerDB),
		s.logger.Child("modelsecretbackend"),
	)
	return secretbackendservice.NewModelSecretBackendService(s.modelUUID, state)
}

// Agent returns the model's agent service.
func (s *ModelServices) Agent() *modelagentservice.WatchableService {
	return modelagentservice.NewWatchableService(
		modelagentservice.DefaultAgentBinaryFinder(),
		modelagentstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.modelWatcherFactory("modelagent"),
	)
}

// ModelInfo returns the model info service.
func (s *ModelServices) ModelInfo() *modelservice.ProviderModelService {
	return modelservice.NewProviderModelService(
		s.modelUUID,
		modelstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		modelstate.NewModelState(changestream.NewTxnRunnerFactory(s.modelDB), s.logger.Child("modelinfo")),
		modelservice.EnvironVersionProviderGetter(),
		providertracker.ProviderRunner[modelservice.ModelResourcesProvider](s.providerFactory, s.modelUUID.String()),
		providertracker.ProviderRunner[modelservice.CloudInfoProvider](s.providerFactory, s.modelUUID.String()),
		modelservice.DefaultAgentBinaryFinder(),
	)
}

// Proxy returns the proxy service.
func (s *ModelServices) Proxy() *proxy.Service {
	return proxy.NewService(
		providertracker.ProviderRunner[proxy.Provider](s.providerFactory, s.modelUUID.String()),
	)
}

// UnitState returns the service for persisting and retrieving remote unit
// state. This is used to reconcile with local state to determine which
// hooks to run, and is saved upon hook completion.
func (s *ModelServices) UnitState() *unitstateservice.Service {
	return unitstateservice.NewService(
		unitstatestate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
	)
}

// CloudImageMetadata returns the service for persisting and retrieving cloud
// image metadata for the current model.
func (s *ModelServices) CloudImageMetadata() *cloudimagemetadataservice.Service {
	return cloudimagemetadataservice.NewService(
		cloudimagemetadatastate.NewState(
			changestream.NewTxnRunnerFactory(s.controllerDB),
			s.clock,
			s.logger.Child("cloudimagemetadata"),
		),
	)
}

// Port returns the service for managing opened port ranges for units.
func (s *ModelServices) Port() *portservice.WatchableService {
	return portservice.NewWatchableService(
		portstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.modelWatcherFactory("port"),
		s.logger.Child("port"),
	)
}

// BlockCommand returns the service for blocking commands.
func (s *ModelServices) BlockCommand() *blockcommandservice.Service {
	return blockcommandservice.NewService(
		blockcommandstate.NewState(changestream.NewTxnRunnerFactory(s.modelDB)),
		s.logger.Child("blockcommand"),
	)
}

// Resource returns the service for persisting and retrieving application
// resources for the current model.
func (s *ModelServices) Resource() *resourceservice.Service {
	containerImageResourceStoreGetter := func() coreresourcestore.ResourceStore {
		return containerimageresourcestoreservice.NewService(
			containerimageresourcestorestate.NewState(
				changestream.NewTxnRunnerFactory(s.modelDB),
				s.logger.Child("containerimageresourcestore.state"),
			),
			s.logger.Child("containerimageresourcestore.service"),
		)
	}
	resourceStoreFactory := store.NewResourceStoreFactory(
		s.modelObjectStoreGetter,
		containerImageResourceStoreGetter,
	)
	return resourceservice.NewService(
		resourcestate.NewState(
			changestream.NewTxnRunnerFactory(s.modelDB),
			s.clock,
			s.logger.Child("resource.state"),
		),
		resourceStoreFactory,
		s.logger.Child("resource.service"),
	)
}

// Relation returns the service for persisting and retrieving relations
// for the current model.
func (s *ModelServices) Relation() *relationservice.WatchableService {
	return relationservice.NewWatchableService(
		relationstate.NewState(
			changestream.NewTxnRunnerFactory(s.modelDB),
			s.clock,
			s.logger.Child("relation.state"),
		),
		s.modelWatcherFactory("relation.watcher"),
		domain.NewLeaseService(s.leaseManager),
		s.logger.Child("relation.service"),
	)
}

// Removal returns the service for working
// with entity removals in the current model.
func (s *ModelServices) Removal() *removalservice.WatchableService {
	log := s.logger.Child("removal")

	return removalservice.NewWatchableService(
		removalstatecontroller.NewState(changestream.NewTxnRunnerFactory(s.controllerDB), log),
		removalstatemodel.NewState(changestream.NewTxnRunnerFactory(s.modelDB), log),
		s.modelWatcherFactory("removal"),
		domain.NewLeaseService(s.leaseManager),
		providertracker.ProviderRunner[removalservice.Provider](s.providerFactory, s.modelUUID.String()),
		s.clock,
		log,
	)
}

// AgentPassword returns the service for working with agent passwords in the
// current model.
func (s *ModelServices) AgentPassword() *agentpasswordservice.Service {
	return agentpasswordservice.NewService(
		agentpasswordstate.NewModelState(changestream.NewTxnRunnerFactory(s.modelDB)),
		agentpasswordstate.NewControllerState(changestream.NewTxnRunnerFactory(s.controllerDB)),
	)
}

// ModelProvider returns a service for accessing info relevant to the
// provider for a model.
func (s *ModelServices) ModelProvider() *modelproviderservice.Service {
	return modelproviderservice.NewService(
		s.modelUUID,
		modelproviderstate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.logger.Child("modelprovider"),
		providertracker.ProviderRunner[modelproviderservice.ProviderWithSecretToken](s.providerFactory, s.modelUUID.String()),
	)
}

// Stub returns the stub service. A special service which collects temporary
// methods required to wire together domains which are not completely implemented
// or wired up.
//
// *** ADD NEW METHODS ABOVE THIS, NOT BELOW.
//
// Deprecated: Stub service contains only temporary methods and should be removed
// as soon as possible.
func (s *ModelServices) Stub() *stubservice.StubService {
	return stubservice.NewStubService(
		s.modelUUID,
		changestream.NewTxnRunnerFactory(s.controllerDB),
		changestream.NewTxnRunnerFactory(s.modelDB),
	)
}
