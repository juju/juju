// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/core/model"
	accessservice "github.com/juju/juju/domain/access/service"
	agentprovisionerservice "github.com/juju/juju/domain/agentprovisioner/service"
	annotationservice "github.com/juju/juju/domain/annotation/service"
	applicationservice "github.com/juju/juju/domain/application/service"
	autocertcacheservice "github.com/juju/juju/domain/autocert/service"
	blockdeviceservice "github.com/juju/juju/domain/blockdevice/service"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	controllerservice "github.com/juju/juju/domain/controller/service"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	controllerproxyservice "github.com/juju/juju/domain/controllerproxy/service"
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
	networkservice "github.com/juju/juju/domain/network/service"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	storageservice "github.com/juju/juju/domain/storage/service"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/storage"
)

// TestingServiceFactory provides access to the services required by the apiserver.
type TestingServiceFactory struct {
	machineServiceGetter     func() *machineservice.WatchableService
	applicationServiceGetter func() *applicationservice.WatchableService
}

// NewTestingServiceFactory returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewTestingServiceFactory() *TestingServiceFactory {
	return &TestingServiceFactory{}
}

// AgentProvisioner returns the agent provisioner service.
func (s *TestingServiceFactory) AgentProvisioner() *agentprovisionerservice.Service {
	return nil
}

// AutocertCache returns the autocert cache service.
func (s *TestingServiceFactory) AutocertCache() *autocertcacheservice.Service {
	return nil
}

// Config returns the model config service.
func (s *TestingServiceFactory) Config() *modelconfigservice.WatchableService {
	return nil
}

// Controller returns the controller service.
func (s *TestingServiceFactory) Controller() *controllerservice.Service {
	return nil
}

// ControllerConfig returns the controller configuration service.
func (s *TestingServiceFactory) ControllerConfig() *controllerconfigservice.WatchableService {
	return nil
}

// ControllerNode returns the controller node service.
func (s *TestingServiceFactory) ControllerNode() *controllernodeservice.Service {
	return nil
}

// Model returns the model service.
func (s *TestingServiceFactory) Model() *modelservice.Service {
	return nil
}

// ModelDefaults returns the model defaults service.
func (s *TestingServiceFactory) ModelDefaults() *modeldefaultsservice.Service {
	return nil
}

// KeyManager returns the model key manager service.
func (s *TestingServiceFactory) KeyManager() *keymanagerservice.Service {
	return nil
}

// KeyManagerWithImporter returns the model key manager serivce that is capable
// of importing keys from an external source.
func (s *TestingServiceFactory) KeyManagerWithImporter(_ keymanagerservice.PublicKeyImporter) *keymanagerservice.ImporterService {
	return nil
}

// KeyUpdater returns the model key updater service.
func (s *TestingServiceFactory) KeyUpdater() *keyupdaterservice.WatchableService {
	return nil
}

// ExternalController returns the external controller service.
func (s *TestingServiceFactory) ExternalController() *externalcontrollerservice.WatchableService {
	return nil
}

// Credential returns the credential service.
func (s *TestingServiceFactory) Credential() *credentialservice.WatchableService {
	return nil
}

// Cloud returns the cloud service.
func (s *TestingServiceFactory) Cloud() *cloudservice.WatchableService {
	return nil
}

// Upgrade returns the upgrade service.
func (s *TestingServiceFactory) Upgrade() *upgradeservice.WatchableService {
	return nil
}

// AgentObjectStore returns the agent object store service.
func (s *TestingServiceFactory) AgentObjectStore() *objectstoreservice.WatchableService {
	return nil
}

// ObjectStore returns the object store service.
func (s *TestingServiceFactory) ObjectStore() *objectstoreservice.WatchableService {
	return nil
}

// Flag returns the flag service.
func (s *TestingServiceFactory) Flag() *flagservice.Service {
	return nil
}

// Access returns the access service.
func (s *TestingServiceFactory) Access() *accessservice.Service {
	return nil
}

// Machine returns the machine service.
func (s *TestingServiceFactory) Machine() *machineservice.WatchableService {
	if s.machineServiceGetter == nil {
		return nil
	}
	return s.machineServiceGetter()
}

// Network returns the network service.
func (s *TestingServiceFactory) Network() *networkservice.WatchableService {
	return nil
}

// Annotation returns the annotation service.
func (s *TestingServiceFactory) Annotation() *annotationservice.Service {
	return nil
}

// Storage returns the storage service.
func (s *TestingServiceFactory) Storage(storage.ProviderRegistry) *storageservice.Service {
	return nil
}

// Secret returns the secret service.
func (s *TestingServiceFactory) Secret(secretservice.BackendAdminConfigGetter) *secretservice.WatchableService {
	return nil
}

// Agent returns the modelagent service.
func (s *TestingServiceFactory) Agent() *modelagentservice.ModelService {
	return nil
}

// Macaroon returns the macaroon bakery service.
func (s *TestingServiceFactory) Macaroon() *macaroonservice.Service {
	return nil
}

// ModelMigration returns the model migration service.
func (s *TestingServiceFactory) ModelMigration() *modelmigrationservice.Service {
	return nil
}

// FactoryForModel returns a service factory for the given model uuid.
// This will late bind the model service factory to the actual service factory.
func (s *TestingServiceFactory) FactoryForModel(modelID model.UUID) servicefactory.ServiceFactory {
	return s
}

// WithMachineService returns a service factory which gets its machine service
// using the supplied getter.
func (s *TestingServiceFactory) WithMachineService(getter func() *machineservice.WatchableService) *TestingServiceFactory {
	s.machineServiceGetter = getter
	return s
}

// BlockDevice returns the block device service.
func (s *TestingServiceFactory) BlockDevice() *blockdeviceservice.WatchableService {
	return nil
}

// SecretBackend returns the secret backend service.
func (s *TestingServiceFactory) SecretBackend() *secretbackendservice.WatchableService {
	return nil
}

// ModelSecretBackend returns the model secret backend service.
func (s *TestingServiceFactory) ModelSecretBackend() *secretbackendservice.ModelSecretBackendService {
	return nil
}

// Application returns the application service.
func (s *TestingServiceFactory) Application(applicationservice.ApplicationServiceParams) *applicationservice.WatchableService {
	if s.applicationServiceGetter == nil {
		return nil
	}
	return s.applicationServiceGetter()
}

// WithApplicationService returns a service factory which gets its application service
// using the supplied getter.
func (s *TestingServiceFactory) WithApplicationService(getter func() *applicationservice.WatchableService) *TestingServiceFactory {
	s.applicationServiceGetter = getter
	return s
}

// ModelInfo returns the model service for the model. The model info
// contains read-only information about the model.
// Note: This should be called model, but we have naming conflicts with
// the model service. As this is only for read-only model information, we
// can rename it to the more obscure version.
func (s *TestingServiceFactory) ModelInfo() *modelservice.ModelService {
	return nil
}

func (s *TestingServiceFactory) ControllerProxy() *controllerproxyservice.Service {
	return nil
}
