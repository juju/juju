// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	modelmanagerservice "github.com/juju/juju/domain/modelmanager/service"
)

// TestRegistry provides access to the services required by the apiserver.
type TestRegistry struct{}

// NewTestRegistry returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewTestRegistry() *TestRegistry {
	return &TestRegistry{}
}

// ControllerConfig returns the controller configuration service.
func (s *TestRegistry) ControllerConfig() *controllerconfigservice.Service {
	return nil
}

// ControllerNode returns the controller node service.
func (s *TestRegistry) ControllerNode() *controllernodeservice.Service {
	return nil
}

// ModelManager returns the model manager service.
func (s *TestRegistry) ModelManager() *modelmanagerservice.Service {
	return nil
}

// ExternalController returns the external controller service.
func (s *TestRegistry) ExternalController() *externalcontrollerservice.Service {
	return nil
}
