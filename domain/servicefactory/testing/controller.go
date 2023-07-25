// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	modelmanagerservice "github.com/juju/juju/domain/modelmanager/service"
)

// TestingControllerFactory provides access to the services required by the apiserver.
type TestingControllerFactory struct{}

// NewTestingControllerFactory returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewTestingControllerFactory() *TestingControllerFactory {
	return &TestingControllerFactory{}
}

// ControllerConfig returns the controller configuration service.
func (s *TestingControllerFactory) ControllerConfig() *controllerconfigservice.Service {
	return nil
}

// ControllerNode returns the controller node service.
func (s *TestingControllerFactory) ControllerNode() *controllernodeservice.Service {
	return nil
}

// ModelManager returns the model manager service.
func (s *TestingControllerFactory) ModelManager() *modelmanagerservice.Service {
	return nil
}

// ExternalController returns the external controller service.
func (s *TestingControllerFactory) ExternalController() *externalcontrollerservice.Service {
	return nil
}
