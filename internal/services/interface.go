// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/juju/core/model"
	accessservice "github.com/juju/juju/domain/access/service"
	agentbinaryservice "github.com/juju/juju/domain/agentbinary/service"
	agentpasswordservice "github.com/juju/juju/domain/agentpassword/service"
	agentprovisionerservice "github.com/juju/juju/domain/agentprovisioner/service"
	annotationService "github.com/juju/juju/domain/annotation/service"
	applicationservice "github.com/juju/juju/domain/application/service"
	autocertcacheservice "github.com/juju/juju/domain/autocert/service"
	blockcommandservice "github.com/juju/juju/domain/blockcommand/service"
	blockdeviceservice "github.com/juju/juju/domain/blockdevice/service"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	cloudimagemetadataservice "github.com/juju/juju/domain/cloudimagemetadata/service"
	controllerservice "github.com/juju/juju/domain/controller/service"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	credentialservice "github.com/juju/juju/domain/credential/service"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	flagservice "github.com/juju/juju/domain/flag/service"
	keymanagerservice "github.com/juju/juju/domain/keymanager/service"
	keyupdaterservice "github.com/juju/juju/domain/keyupdater/service"
	macaroonservice "github.com/juju/juju/domain/macaroon/service"
	machineservice "github.com/juju/juju/domain/machine/service"
	modelservice "github.com/juju/juju/domain/model/service"
	modelagentservice "github.com/juju/juju/domain/modelagent/service"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	modelmigrationservice "github.com/juju/juju/domain/modelmigration/service"
	modelproviderservice "github.com/juju/juju/domain/modelprovider/service"
	networkservice "github.com/juju/juju/domain/network/service"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	portservice "github.com/juju/juju/domain/port/service"
	proxyservice "github.com/juju/juju/domain/proxy/service"
	relationservice "github.com/juju/juju/domain/relation/service"
	removalservice "github.com/juju/juju/domain/removal/service"
	resolveservice "github.com/juju/juju/domain/resolve/service"
	resourceservice "github.com/juju/juju/domain/resource/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	statusservice "github.com/juju/juju/domain/status/service"
	storageservice "github.com/juju/juju/domain/storage/service"
	storageprovisioningservice "github.com/juju/juju/domain/storageprovisioning/service"
	stubservice "github.com/juju/juju/domain/stub"
	unitstateservice "github.com/juju/juju/domain/unitstate/service"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
)

// ControllerDomainServices provides access to the services required by the
// apiserver.
type ControllerDomainServices interface {
	// ControllerAgentBinaryStore returns the agent binary store for the entire
	// controller.
	ControllerAgentBinaryStore() *agentbinaryservice.AgentBinaryStore
	// Controller returns the controller service.
	Controller() *controllerservice.Service
	// ControllerConfig returns the controller configuration service.
	ControllerConfig() *controllerconfigservice.WatchableService
	// ControllerNode returns the controller node service.
	ControllerNode() *controllernodeservice.WatchableService
	// Model returns the model service.
	Model() *modelservice.WatchableService
	//ModelDefaults returns the modeldefaults service.
	ModelDefaults() *modeldefaultsservice.Service
	// ExternalController returns the external controller service.
	ExternalController() *externalcontrollerservice.WatchableService
	// Credential returns the credential service.
	Credential() *credentialservice.WatchableService
	// AutocertCache returns the autocert cache service.
	AutocertCache() *autocertcacheservice.Service
	// Cloud returns the cloud service.
	Cloud() *cloudservice.WatchableService
	// Upgrade returns the upgrade service.
	Upgrade() *upgradeservice.WatchableService
	// Flag returns the flag service.
	Flag() *flagservice.Service
	// Access returns the access service. This includes the user and permission
	// controller.
	Access() *accessservice.Service
	// SecretBackend returns the secret backend service.
	SecretBackend() *secretbackendservice.WatchableService
	// Macaroon returns the macaroon bakery backend service
	Macaroon() *macaroonservice.Service
}

// ModelDomainServices provides access to the services required by the
// apiserver for a given model.
type ModelDomainServices interface {
	// Agent returns the model's agent service.
	Agent() *modelagentservice.WatchableService
	// AgentProvisioner returns the agent provisioner service.
	AgentProvisioner() *agentprovisionerservice.Service
	// AgentBinary returns the agent binary service for the model.
	AgentBinary() *agentbinaryservice.AgentBinaryService
	// AgentBinaryStore returns the agent binary store for the current model.
	AgentBinaryStore() *agentbinaryservice.AgentBinaryStore
	// Annotation returns the annotation service.
	Annotation() *annotationService.Service
	// Config returns the model config service.
	Config() *modelconfigservice.WatchableService
	// Machine returns the machine service.
	Machine() *machineservice.WatchableService
	// BlockDevice returns the block device service.
	BlockDevice() *blockdeviceservice.WatchableService
	// Application returns the application service.
	Application() *applicationservice.WatchableService
	// Status returns the application status service.
	Status() *statusservice.LeadershipService
	// Resolve returns the resolve service.
	Resolve() *resolveservice.WatchableService
	// KeyManager returns the key manager service.
	KeyManager() *keymanagerservice.Service
	// KeyManagerWithImporter returns they manager service that is capable of
	// importing keys from an external source.
	KeyManagerWithImporter() *keymanagerservice.ImporterService
	// KeyUpdater returns the key updater service.
	KeyUpdater() *keyupdaterservice.WatchableService
	// Network returns the space service.
	Network() *networkservice.WatchableService
	// Storage returns the storage service.
	Storage() *storageservice.Service
	// StorageProvisioning returns the storage provisioning service.
	StorageProvisioning() *storageprovisioningservice.Service
	// Secret returns the secret service.
	Secret() *secretservice.WatchableService
	// ModelInfo returns the model service for the model.
	// Note: This should be called model, but we have naming conflicts with
	// the model service. As this is only for model information, we
	// can rename it to the more obscure version.
	ModelInfo() *modelservice.ProviderModelService
	// ModelMigration returns the model's migration service for support
	// migration operations.
	ModelMigration() *modelmigrationservice.Service
	// ModelSecretBackend returns the model secret backend service.
	ModelSecretBackend() *secretbackendservice.ModelSecretBackendService
	// Proxy returns the proxy service.
	Proxy() *proxyservice.Service
	// UnitState returns the service for persisting and retrieving remote unit
	// state. This is used to reconcile with local state to determine which
	// hooks to run, and is saved upon hook completion.
	UnitState() *unitstateservice.Service
	// CloudImageMetadata returns the service for persisting and retrieving
	// cloud image metadata for a specific model.
	CloudImageMetadata() *cloudimagemetadataservice.Service
	// Port returns the service for managing opened port ranges for units.
	Port() *portservice.WatchableService
	// BlockCommand returns the service for blocking commands.
	BlockCommand() *blockcommandservice.Service
	// Relation returns the service for managing relations.
	Relation() *relationservice.WatchableService
	// Resource returns the service for managing resources.
	Resource() *resourceservice.Service
	// Removal returns the service for managing entity removal.
	Removal() *removalservice.WatchableService
	// AgentPassword returns the service for managing agent passwords.
	AgentPassword() *agentpasswordservice.Service
	// ModelProvider returns a service for accessing info relevant to the
	// provider for a model.
	ModelProvider() *modelproviderservice.Service

	// Stub returns the stub service. A special service that collects temporary
	// methods required for wiring together domains which are not completely
	// implemented or wired up.
	//
	// Deprecated: Stub service contains only temporary methods and should be removed
	// as soon as possible.
	Stub() *stubservice.StubService
}

// DomainServices provides access to the services required by the apiserver.
type DomainServices interface {
	ControllerDomainServices
	ModelDomainServices
}

// DomainServicesGetter represents a way to get a DomainServices for a given
// model.
type DomainServicesGetter interface {
	// ServicesForModel returns a DomainServices for the given model.
	ServicesForModel(ctx context.Context, modelID model.UUID) (DomainServices, error)
}

// ProviderServices provides access to the services required by the
// provider.
type ProviderServices interface {
	// Model returns the provider model service.
	Model() *modelservice.ProviderService
	// Cloud returns the provider cloud service.
	Cloud() *cloudservice.WatchableProviderService
	// Config returns the provider config service.
	Config() *modelconfigservice.WatchableProviderService
	// Credential returns the provider credential service.
	Credential() *credentialservice.WatchableProviderService
}

// ProviderServicesGetter represents a way to get a ProviderServices
// for a given model.
type ProviderServicesGetter interface {
	// ServicesForModel returns a ProviderServices for the given model.
	ServicesForModel(modelUUID string) ProviderServices
}

// ControllerObjectStoreServices provides access to the services required by the
// apiserver.
// This is a subset of the ObjectStoreServices interface, for use only be
// object store workers, that want to operate in a controller context. Think
// s3caller, which wants the controller config service. We could use the
// controller domain services, but that would re-introduce a circular
// dependency. This isn't pretty, but is a necessary evil.
type ControllerObjectStoreServices interface {
	// ControllerConfig returns the controller configuration service.
	ControllerConfig() *controllerconfigservice.WatchableService

	// AgentObjectStore returns the object store service.
	// Primarily used for agent blob store. Although can be used for other
	// blob related operations.
	AgentObjectStore() *objectstoreservice.WatchableDrainingService
}

// ObjectStoreServices provides access to the services required by the
// apiserver.
type ObjectStoreServices interface {
	ControllerObjectStoreServices

	// ObjectStore returns the object store service.
	ObjectStore() *objectstoreservice.WatchableService
}

// ObjectStoreServicesGetter represents a way to get a ObjectStoreServices
// for a given model.
type ObjectStoreServicesGetter interface {
	// ServicesForModel returns a ObjectStoreServices for the given model.
	ServicesForModel(modelUUID model.UUID) ObjectStoreServices
}
