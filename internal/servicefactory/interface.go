// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	accessservice "github.com/juju/juju/domain/access/service"
	annotationService "github.com/juju/juju/domain/annotation/service"
	applicationservice "github.com/juju/juju/domain/application/service"
	autocertcacheservice "github.com/juju/juju/domain/autocert/service"
	blockdeviceservice "github.com/juju/juju/domain/blockdevice/service"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	credentialservice "github.com/juju/juju/domain/credential/service"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	flagservice "github.com/juju/juju/domain/flag/service"
	machineservice "github.com/juju/juju/domain/machine/service"
	modelservice "github.com/juju/juju/domain/model/service"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	networkservice "github.com/juju/juju/domain/network/service"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	storageservice "github.com/juju/juju/domain/storage/service"
	unitservice "github.com/juju/juju/domain/unit/service"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	"github.com/juju/juju/internal/storage"
)

// ControllerServiceFactory provides access to the services required by the
// apiserver.
type ControllerServiceFactory interface {
	// ControllerConfig returns the controller configuration service.
	ControllerConfig() *controllerconfigservice.WatchableService
	// ControllerNode returns the controller node service.
	ControllerNode() *controllernodeservice.Service
	// Model returns the model service.
	Model() *modelservice.Service
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
	// AgentObjectStore returns the object store service.
	// Primarily used for agent blob store. Although can be used for other
	// blob related operations.
	AgentObjectStore() *objectstoreservice.WatchableService
	// Flag returns the flag service.
	Flag() *flagservice.Service
	// Access returns the access service. This includes the user and permission
	// controller.
	Access() *accessservice.Service
	// SecretBackend returns the secret backend service.
	SecretBackend(string, secretbackendservice.SecretProviderRegistry) *secretbackendservice.WatchableService
}

// ModelServiceFactory provides access to the services required by the
// apiserver for a given model.
type ModelServiceFactory interface {
	// Config returns the modelconfig service.
	Config(modelconfigservice.ModelDefaultsProvider) *modelconfigservice.WatchableService
	// ObjectStore returns the object store service.
	ObjectStore() *objectstoreservice.WatchableService
	// Machine returns the machine service.
	Machine() *machineservice.Service
	// BlockDevice returns the block device service.
	BlockDevice() *blockdeviceservice.WatchableService
	// Application returns the application service.
	Application(registry storage.ProviderRegistry) *applicationservice.Service
	// Unit returns the machine service.
	Unit() *unitservice.Service
	// Network returns the space service.
	Network() *networkservice.Service
	// Annotation returns the annotation service.
	Annotation() *annotationService.Service
	// Storage returns the storage service.
	Storage(registry storage.ProviderRegistry) *storageservice.Service
	// Secret returns the secret service.
	Secret(secretservice.BackendAdminConfigGetter) *secretservice.SecretService
	// ModelInfo returns the model service for the model. The model info
	// contains read-only information about the model.
	// Note: This should be called model, but we have naming conflicts with
	// the model service. As this is only for read-only model information, we
	// can rename it to the more obscure version.
	ModelInfo() *modelservice.ModelService
}

// ServiceFactory provides access to the services required by the apiserver.
type ServiceFactory interface {
	ControllerServiceFactory
	ModelServiceFactory
}

// ServiceFactoryGetter represents a way to get a ServiceFactory for a given
// model.
type ServiceFactoryGetter interface {
	// FactoryForModel returns a ServiceFactory for the given model.
	FactoryForModel(modelUUID string) ServiceFactory
}

// ProviderServiceFactory provides access to the services required by the
// provider.
type ProviderServiceFactory interface {
	// Model returns the provider model service.
	Model() *modelservice.ProviderService
	// Cloud returns the provider cloud service.
	Cloud() *cloudservice.WatchableProviderService
	// Config returns the provider config service.
	Config() *modelconfigservice.WatchableProviderService
	// Credential returns the provider credential service.
	Credential() *credentialservice.WatchableProviderService
}

// ProviderServiceFactoryGetter represents a way to get a ProviderServiceFactory
// for a given model.
type ProviderServiceFactoryGetter interface {
	// FactoryForModel returns a ProviderServiceFactory for the given model.
	FactoryForModel(modelUUID string) ProviderServiceFactory
}
