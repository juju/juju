// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
)

// ControllerConfigService is the interface that the worker uses to get the
// controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelService is the interface that the worker uses to get model information.
type ModelService interface {
	// ControllerModel returns information for the controller model.
	ControllerModel(context.Context) (model.Model, error)
}
