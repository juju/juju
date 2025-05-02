// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
)

// ControllerConfigService is the interface that the worker uses to get the
// controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(context.Context) (controller.Config, error)
	// Watch returns a watcher that returns keys for any changes to controller
	// config.
	WatchControllerConfig() (watcher.StringsWatcher, error)
}

// ModelService is the interface that the worker uses to get model information.
type ModelService interface {
	// ControllerModel returns information for the controller model.
	ControllerModel(context.Context) (model.Model, error)
}
