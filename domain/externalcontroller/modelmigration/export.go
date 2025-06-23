// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/externalcontroller/service"
	"github.com/juju/juju/domain/externalcontroller/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator) {
	coordinator.Add(&exportOperation{})
}

// ExportService provides a subset of the external controller domain
// service methods needed for external controller export.
type ExportService interface {
	// ControllerForModel returns the controller record that's associated
	// with the modelUUID.
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)

	// ModelsForController returns the list of model UUIDs for
	// the given controllerUUID.
	ModelsForController(ctx context.Context, controllerUUID string) ([]string, error)

	// ControllersForModels returns the list of controllers which
	// are part of the given modelUUIDs.
	// The resulting MigrationControllerInfo contains the list of models
	// for each controller.
	ControllersForModels(ctx context.Context, modelUUIDs ...string) ([]crossmodel.ControllerInfo, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export external controllers"
}

func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	e.service = service.NewService(
		state.NewState(scope.ControllerDB()))
	return nil
}

// Execute the migration of the external controllers using typed interfaces, to
// ensure we don't loose any type safety.
// This export makes use of the remote applications on the source model. Since
// the source model is stored in mongodb whilst the destination (external)
// controller is stored in dqlite, we need to be very careful when we fill
// the dst description.Model argument and make sure that the model is not
// updated until the export has finished, thus avoiding a race on the
// remote applications of the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	// Get the list of model UUIDs from the remote applications. We don't
	// care that the modelUUIDs might be repeated among different remote
	// applications, since the db query takes a list of modelUUIDs so there
	// is no extra performance cost.
	var sourceModelUUIDs []string
	for _, remoteApp := range model.RemoteApplications() {
		sourceModelUUIDs = append(sourceModelUUIDs, remoteApp.SourceModelUUID())
	}

	controllers, err := e.service.ControllersForModels(ctx, sourceModelUUIDs...)
	if err != nil {
		return errors.Capture(err)
	}

	for _, controller := range controllers {
		_ = model.AddExternalController(description.ExternalControllerArgs{
			ID:     controller.ControllerUUID,
			Addrs:  controller.Addrs,
			Alias:  controller.Alias,
			CACert: controller.CACert,
			Models: controller.ModelUUIDs,
		})
	}

	return nil
}
