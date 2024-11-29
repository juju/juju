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
	networkservice "github.com/juju/juju/domain/network/service"
	portservice "github.com/juju/juju/domain/port/service"
	proxyservice "github.com/juju/juju/domain/proxy/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	storageservice "github.com/juju/juju/domain/storage/service"
	stubservice "github.com/juju/juju/domain/stub"
	unitstateservice "github.com/juju/juju/domain/unitstate/service"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	"github.com/juju/juju/internal/services"
)

type placeholderDomainServices struct{}

// NewPlaceholderDomainServices returns a new registry which can be used as
// a placeholder for tests which are required to provide a non-nil registry.
// Asking the registry for services will return nil values so be careful
// not to expect the registry to be of any use other than as a placeholder.
func NewPlaceholderDomainServices() *placeholderDomainServices {
	return &placeholderDomainServices{}
}

// AgentProvisioner returns the agent provisioner service.
func (s *placeholderDomainServices) AgentProvisioner() *agentprovisionerservice.Service {
	return nil
}

// AutocertCache returns the autocert cache service.
func (s *placeholderDomainServices) AutocertCache() *autocertcacheservice.Service {
	return nil
}

// Config returns the model config service.
func (s *placeholderDomainServices) Config() *modelconfigservice.WatchableService {
	return nil
}

// Controller returns the controller service.
func (s *placeholderDomainServices) Controller() *controllerservice.Service {
	return nil
}

// ControllerConfig returns the controller configuration service.
func (s *placeholderDomainServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return nil
}

// ControllerNode returns the controller node service.
func (s *placeholderDomainServices) ControllerNode() *controllernodeservice.Service {
	return nil
}

// Model returns the model service.
func (s *placeholderDomainServices) Model() *modelservice.Service {
	return nil
}

// ModelDefaults returns the model defaults service.
func (s *placeholderDomainServices) ModelDefaults() *modeldefaultsservice.Service {
	return nil
}

// KeyManager returns the model key manager service.
func (s *placeholderDomainServices) KeyManager() *keymanagerservice.Service {
	return nil
}

// KeyManagerWithImporter returns the model key manager serivce that is capable
// of importing keys from an external source.
func (s *placeholderDomainServices) KeyManagerWithImporter() *keymanagerservice.ImporterService {
	return nil
}

// KeyUpdater returns the model key updater service.
func (s *placeholderDomainServices) KeyUpdater() *keyupdaterservice.WatchableService {
	return nil
}

// ExternalController returns the external controller service.
func (s *placeholderDomainServices) ExternalController() *externalcontrollerservice.WatchableService {
	return nil
}

// Credential returns the credential service.
func (s *placeholderDomainServices) Credential() *credentialservice.WatchableService {
	return nil
}

// Cloud returns the cloud service.
func (s *placeholderDomainServices) Cloud() *cloudservice.WatchableService {
	return nil
}

// Upgrade returns the upgrade service.
func (s *placeholderDomainServices) Upgrade() *upgradeservice.WatchableService {
	return nil
}

// Flag returns the flag service.
func (s *placeholderDomainServices) Flag() *flagservice.Service {
	return nil
}

// Access returns the access service.
func (s *placeholderDomainServices) Access() *accessservice.Service {
	return nil
}

// Machine returns the machine service.
func (s *placeholderDomainServices) Machine() *machineservice.WatchableService {
	return nil
}

// Network returns the network service.
func (s *placeholderDomainServices) Network() *networkservice.WatchableService {
	return nil
}

// Annotation returns the annotation service.
func (s *placeholderDomainServices) Annotation() *annotationservice.Service {
	return nil
}

// Storage returns the storage service.
func (s *placeholderDomainServices) Storage() *storageservice.Service {
	return nil
}

// Secret returns the secret service.
func (s *placeholderDomainServices) Secret(secretservice.SecretServiceParams) *secretservice.WatchableService {
	return nil
}

// Agent returns the modelagent service.
func (s *placeholderDomainServices) Agent() *modelagentservice.ModelService {
	return nil
}

// Macaroon returns the macaroon bakery service.
func (s *placeholderDomainServices) Macaroon() *macaroonservice.Service {
	return nil
}

// ModelMigration returns the model migration service.
func (s *placeholderDomainServices) ModelMigration() *modelmigrationservice.Service {
	return nil
}

// ServicesForModel returns a domain services for the given model uuid.
// This will late bind the model domain services to the actual domain services.
func (s *placeholderDomainServices) ServicesForModel(modelID model.UUID) services.DomainServices {
	return s
}

// BlockDevice returns the block device service.
func (s *placeholderDomainServices) BlockDevice() *blockdeviceservice.WatchableService {
	return nil
}

// SecretBackend returns the secret backend service.
func (s *placeholderDomainServices) SecretBackend() *secretbackendservice.WatchableService {
	return nil
}

// ModelSecretBackend returns the model secret backend service.
func (s *placeholderDomainServices) ModelSecretBackend() *secretbackendservice.ModelSecretBackendService {
	return nil
}

// Application returns the application service.
func (s *placeholderDomainServices) Application() *applicationservice.WatchableService {
	return nil
}

// ModelInfo returns the model service for the model. The model info
// contains read-only information about the model.
// Note: This should be called model, but we have naming conflicts with
// the model service. As this is only for read-only model information, we
// can rename it to the more obscure version.
func (s *placeholderDomainServices) ModelInfo() *modelservice.ModelService {
	return nil
}

func (s *placeholderDomainServices) Proxy() *proxyservice.Service {
	return nil
}

func (s *placeholderDomainServices) UnitState() *unitstateservice.Service {
	return nil
}

// CloudImageMetadata returns the service for persisting and retrieving cloud image metadata for the current model.
func (s *placeholderDomainServices) CloudImageMetadata() *cloudimagemetadataservice.Service {
	return nil
}

func (s *placeholderDomainServices) Port() *portservice.WatchableService {
	return nil
}

func (s *placeholderDomainServices) Stub() *stubservice.StubService {
	return nil
}

func (s *placeholderDomainServices) BlockCommand() *blockcommandservice.Service {
	return nil
}
