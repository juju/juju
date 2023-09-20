// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	autocertcacheservice "github.com/juju/juju/domain/autocert/service"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	credentialservice "github.com/juju/juju/domain/credential/service"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	"github.com/juju/juju/domain/model"
	modelservice "github.com/juju/juju/domain/model/service"
	modelmanagerservice "github.com/juju/juju/domain/modelmanager/service"
)

// TestingServiceFactory provides access to the services required by the apiserver.
type TestingServiceFactory struct{}

// NewTestingServiceFactory returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewTestingServiceFactory() *TestingServiceFactory {
	return &TestingServiceFactory{}
}

// AutocertCache returns the autocert cache service.
func (s *TestingServiceFactory) AutocertCache() *autocertcacheservice.Service {
	return nil
}

// ControllerConfig returns the controller configuration service.
func (s *TestingServiceFactory) ControllerConfig() *controllerconfigservice.Service {
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

// ModelManager returns the model manager service.
func (s *TestingServiceFactory) ModelManager() *modelmanagerservice.Service {
	return nil
}

// ModelUUID is the current UUID for the model.
func (s *TestingServiceFactory) ModelUUID() model.UUID {
	return model.UUID("")
}

// ExternalController returns the external controller service.
func (s *TestingServiceFactory) ExternalController() *externalcontrollerservice.Service {
	return nil
}

// Credential returns the credential service.
func (s *TestingServiceFactory) Credential() *credentialservice.Service {
	return nil
}

// Cloud returns the cloud service.
func (s *TestingServiceFactory) Cloud() *cloudservice.Service {
	return nil
}

func (s *TestingServiceFactory) Name() string {
	return "TestingServiceFactory"
}
