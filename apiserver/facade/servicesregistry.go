// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import (
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	externalcontrollerservice "github.com/juju/juju/domain/externalcontroller/service"
	modelmanagerservice "github.com/juju/juju/domain/modelmanager/service"
)

// ServicesRegistry provides access to the services required by the apiserver.
type ServicesRegistry interface {
	// ControllerConfig returns the controller configuration service.
	ControllerConfig() *controllerconfigservice.Service
	// ControllerNode returns the controller node service.
	ControllerNode() *controllernodeservice.Service
	// ModelManager returns the model manager service.
	ModelManager() *modelmanagerservice.Service
	// ExternalController returns the external controller service.
	ExternalController() *externalcontrollerservice.Service
}
