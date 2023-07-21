// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/apiserver/services"
)

// TestRegistry provides access to the services required by the apiserver.
type TestRegistry struct{}

// NewTestRegistry returns a new registry which uses the provided controllerDB
// function to obtain a controller database.
func NewTestRegistry() *TestRegistry {
	return &TestRegistry{}
}

// ControllerConfig returns the controller configuration service.
func (s *TestRegistry) ControllerConfig() services.ControllerConfig {
	return nil
}

// ControllerNode returns the controller node service.
func (s *TestRegistry) ControllerNode() services.ControllerNode {
	return nil
}

// ModelManager returns the model manager service.
func (s *TestRegistry) ModelManager() services.ModelManager {
	return nil
}

// ExternalController returns the external controller service.
func (s *TestRegistry) ExternalController() services.ExternalController {
	return nil
}
