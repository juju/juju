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
	modelmanagerservice "github.com/juju/juju/domain/modelmanager/service"
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
}

// ModelServiceFactory provides access to the services required by the
// apiserver for a given model.
type ModelServiceFactory interface {
	// TODO we need a method here because if we don't have a type here, then
	// anything satisfies the ModelFactory. Once we have model methods here, we
	// can remove this method.
	TODO()
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

// ServiceFactoryGetterFunc is a convenience type for translating a getter
// function into the ServiceFactoryGetter interface.
type ServiceFactoryGetterFunc func(string) ServiceFactory

// FactoryForModel implements the ServiceFactoryGetter interface.
func (s ServiceFactoryGetterFunc) FactoryForModel(modelUUID string) ServiceFactory {
	return s(modelUUID)
}
