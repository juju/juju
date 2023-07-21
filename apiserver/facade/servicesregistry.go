// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import "github.com/juju/juju/apiserver/services"

// ServicesRegistry provides access to the services required by the apiserver.
type ServicesRegistry interface {
	// ControllerConfig returns the controller configuration service.
	ControllerConfig() services.ControllerConfig
	// ControllerNode returns the controller node service.
	ControllerNode() services.ControllerNode
	// ModelManager returns the model manager service.
	ModelManager() services.ModelManager
	// ExternalController returns the external controller service.
	ExternalController() services.ExternalController
}
