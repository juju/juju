// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/watcher"
	modelmanagerservice "github.com/juju/juju/domain/modelmanager/service"
)

// ControllerConfig provides access to the controller configuration.
type ControllerConfig interface {
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(context.Context) (controller.Config, error)
	// UpdateControllerConfig updates the controller configuration.
	UpdateControllerConfig(context.Context, controller.Config, []string) error
	// Watch returns a watcher that notifies of changes to the controller
	// configuration.
	Watch() (watcher.StringsWatcher, error)
}

// ControllerNode provides access to the controller node records.
type ControllerNode interface {
	// CurateNodes modifies the known control plane by adding and removing
	// controller node records according to the input slices.
	CurateNodes(context.Context, []string, []string) error
	// UpdateBootstrapNodeBindAddress sets the input address as the Dqlite
	// bind address of the original bootstrapped controller node.
	UpdateBootstrapNodeBindAddress(context.Context, string) error
	// IsModelKnownToController returns true if the input
	// model UUID is one managed by this controller.
	IsModelKnownToController(context.Context, string) (bool, error)
}

// ModelManager provides access to the model manager.
type ModelManager interface {
	// Create takes a model UUID and creates a new model.
	Create(ctx context.Context, uuid modelmanagerservice.UUID) error
	// Delete takes a model UUID and deletes the model if it exists.
	Delete(ctx context.Context, uuid modelmanagerservice.UUID) error
}

// ExternalController provides access to the external controller records.
type ExternalController interface {
	// Controller returns the controller record.
	Controller(context.Context, string) (*crossmodel.ControllerInfo, error)
	// ControllerForModel returns the controller record that's associated
	// with the modelUUID.
	ControllerForModel(context.Context, string) (*crossmodel.ControllerInfo, error)
	// UpdateExternalController persists the input controller
	// record and associates it with the input model UUIDs.
	UpdateExternalController(context.Context, crossmodel.ControllerInfo) error
	// Watch returns a watcher that observes changes to external controllers.
	Watch() (watcher.StringsWatcher, error)
	// ModelsForController returns the list of model UUIDs for
	// the given controllerUUID.
	ModelsForController(context.Context, string) ([]string, error)
	// ControllersForModels returns the list of controllers which
	// are part of the given modelUUIDs.
	// The resulting MigrationControllerInfo contains the list of models
	// for each controller.
	ControllersForModels(context.Context, ...string) ([]crossmodel.ControllerInfo, error)
}
