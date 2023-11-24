// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	autocertcacheservice "github.com/juju/juju/domain/autocert/service"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	credentialservice "github.com/juju/juju/domain/credential/service"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	modelservice "github.com/juju/juju/domain/model/service"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	modelmanagerservice "github.com/juju/juju/domain/modelmanager/service"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
)

// ControllerServiceFactory provides access to the services required by the
// apiserver.
type ControllerServiceFactory interface {
	// ControllerConfig returns the controller configuration service.
	ControllerConfig() *controllerconfigservice.Service
	// ControllerNode returns the controller node service.
	ControllerNode() *controllernodeservice.Service
	// Model returns the model service.
	Model() *modelservice.Service
	//ModelDefaults returns the modeldefaults service.
	ModelDefaults() *modeldefaultsservice.Service
	// ModelManager returns the model manager service.
	ModelManager() *modelmanagerservice.Service
	// ExternalController returns the external controller service.
	ExternalController() *externalcontrollerservice.Service
	// Credential returns the credential service.
	Credential() *credentialservice.Service
	// AutocertCache returns the autocert cache service.
	AutocertCache() *autocertcacheservice.Service
	// Cloud returns the cloud service.
	Cloud() *cloudservice.Service
	// Upgrade returns the upgrade service.
	Upgrade() *upgradeservice.Service
	// AgentObjectStore returns the object store service.
	// Preliminary used for agent blob store. Although can be used for other
	// blob related operations.
	AgentObjectStore() *objectstoreservice.Service
}

// ModelServiceFactory provides access to the services required by the
// apiserver for a given model.
type ModelServiceFactory interface {
	// Config returns the modelconfig service.
	Config(modelconfigservice.ModelDefaultsProvider) *modelconfigservice.Service
	// ObjectStore returns the object store service.
	ObjectStore() *objectstoreservice.Service
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
