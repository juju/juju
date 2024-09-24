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
	proxyservice "github.com/juju/juju/domain/proxy/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	storageservice "github.com/juju/juju/domain/storage/service"
	unitstateservice "github.com/juju/juju/domain/unitstate/service"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/storage"
)

// TestingDomainServices provides access to the services required by the apiserver.
type TestingDomainServices struct {
	machineServiceGetter     func() *machineservice.WatchableService
	applicationServiceGetter func() *applicationservice.WatchableService
}

// NewTestingDomainServices returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewTestingDomainServices() *TestingDomainServices {
	return &TestingDomainServices{}
}

// AgentProvisioner returns the agent provisioner service.
func (s *TestingDomainServices) AgentProvisioner() *agentprovisionerservice.Service {
	return nil
}

// AutocertCache returns the autocert cache service.
func (s *TestingDomainServices) AutocertCache() *autocertcacheservice.Service {
	return nil
}

// Config returns the model config service.
func (s *TestingDomainServices) Config() *modelconfigservice.WatchableService {
	return nil
}

// Controller returns the controller service.
func (s *TestingDomainServices) Controller() *controllerservice.Service {
	return nil
}

// ControllerConfig returns the controller configuration service.
func (s *TestingDomainServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return nil
}

// ControllerNode returns the controller node service.
func (s *TestingDomainServices) ControllerNode() *controllernodeservice.Service {
	return nil
}

// Model returns the model service.
func (s *TestingDomainServices) Model() *modelservice.Service {
	return nil
}

// ModelDefaults returns the model defaults service.
func (s *TestingDomainServices) ModelDefaults() *modeldefaultsservice.Service {
	return nil
}

// KeyManager returns the model key manager service.
func (s *TestingDomainServices) KeyManager() *keymanagerservice.Service {
	return nil
}

// KeyManagerWithImporter returns the model key manager serivce that is capable
// of importing keys from an external source.
func (s *TestingDomainServices) KeyManagerWithImporter(_ keymanagerservice.PublicKeyImporter) *keymanagerservice.ImporterService {
	return nil
}

// KeyUpdater returns the model key updater service.
func (s *TestingDomainServices) KeyUpdater() *keyupdaterservice.WatchableService {
	return nil
}

// ExternalController returns the external controller service.
func (s *TestingDomainServices) ExternalController() *externalcontrollerservice.WatchableService {
	return nil
}

// Credential returns the credential service.
func (s *TestingDomainServices) Credential() *credentialservice.WatchableService {
	return nil
}

// Cloud returns the cloud service.
func (s *TestingDomainServices) Cloud() *cloudservice.WatchableService {
	return nil
}

// Upgrade returns the upgrade service.
func (s *TestingDomainServices) Upgrade() *upgradeservice.WatchableService {
	return nil
}

// Flag returns the flag service.
func (s *TestingDomainServices) Flag() *flagservice.Service {
	return nil
}

// Access returns the access service.
func (s *TestingDomainServices) Access() *accessservice.Service {
	return nil
}

// Machine returns the machine service.
func (s *TestingDomainServices) Machine() *machineservice.WatchableService {
	if s.machineServiceGetter == nil {
		return nil
	}
	return s.machineServiceGetter()
}

// Network returns the network service.
func (s *TestingDomainServices) Network() *networkservice.WatchableService {
	return nil
}

// Annotation returns the annotation service.
func (s *TestingDomainServices) Annotation() *annotationservice.Service {
	return nil
}

// Storage returns the storage service.
func (s *TestingDomainServices) Storage(storage.ProviderRegistry) *storageservice.Service {
	return nil
}

// Secret returns the secret service.
func (s *TestingDomainServices) Secret(secretservice.SecretServiceParams) *secretservice.WatchableService {
	return nil
}

// Agent returns the modelagent service.
func (s *TestingDomainServices) Agent() *modelagentservice.ModelService {
	return nil
}

// Macaroon returns the macaroon bakery service.
func (s *TestingDomainServices) Macaroon() *macaroonservice.Service {
	return nil
}

// ModelMigration returns the model migration service.
func (s *TestingDomainServices) ModelMigration() *modelmigrationservice.Service {
	return nil
}

// ServicesForModel returns a domain services for the given model uuid.
// This will late bind the model domain services to the actual domain services.
func (s *TestingDomainServices) ServicesForModel(modelID model.UUID) services.DomainServices {
	return s
}

// WithMachineService returns a domain services which gets its machine service
// using the supplied getter.
func (s *TestingDomainServices) WithMachineService(getter func() *machineservice.WatchableService) *TestingDomainServices {
	s.machineServiceGetter = getter
	return s
}

// BlockDevice returns the block device service.
func (s *TestingDomainServices) BlockDevice() *blockdeviceservice.WatchableService {
	return nil
}

// SecretBackend returns the secret backend service.
func (s *TestingDomainServices) SecretBackend() *secretbackendservice.WatchableService {
	return nil
}

// ModelSecretBackend returns the model secret backend service.
func (s *TestingDomainServices) ModelSecretBackend() *secretbackendservice.ModelSecretBackendService {
	return nil
}

// Application returns the application service.
func (s *TestingDomainServices) Application(applicationservice.ApplicationServiceParams) *applicationservice.WatchableService {
	if s.applicationServiceGetter == nil {
		return nil
	}
	return s.applicationServiceGetter()
}

// WithApplicationService returns a domain services which gets its application service
// using the supplied getter.
func (s *TestingDomainServices) WithApplicationService(getter func() *applicationservice.WatchableService) *TestingDomainServices {
	s.applicationServiceGetter = getter
	return s
}

// ModelInfo returns the model service for the model. The model info
// contains read-only information about the model.
// Note: This should be called model, but we have naming conflicts with
// the model service. As this is only for read-only model information, we
// can rename it to the more obscure version.
func (s *TestingDomainServices) ModelInfo() *modelservice.ModelService {
	return nil
}

func (s *TestingDomainServices) Proxy() *proxyservice.Service {
	return nil
}

func (s *TestingDomainServices) UnitState() *unitstateservice.Service {
	return nil
}
