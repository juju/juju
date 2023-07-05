// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v4"
	"github.com/juju/errors"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/externalcontroller"
	"github.com/juju/juju/domain/externalcontroller/service"
	"github.com/juju/juju/domain/externalcontroller/state"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator) {
	coordinator.Add(&exportOperation{})
}

type ExportService interface {
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)
	ModelsForController(ctx context.Context, controllerUUID string) ([]string, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controller s.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
}

func (e exportOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	e.service = service.NewService(
		state.NewState(domain.ConstFactory(scope.ControllerDB())), nil)
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
func (e exportOperation) Execute(ctx context.Context, model description.Model) error {
	// If there are not remote applications, then no external controllers will
	// be exported. We should understand if that's ever going to be an issue?
	remoteApplications := model.RemoteApplications()

	// Iterate over the source model UUIDs, to gather up all the related
	// external controllers. Store them in a map to create a unique set of
	// source model UUIDs, that way we don't request multiple versions of the
	// same external controller.
	sourceModelUUIDs := make(map[string]struct{})
	for _, remoteApp := range remoteApplications {
		sourceModelUUIDs[remoteApp.SourceModelTag().Id()] = struct{}{}
	}

	controllers := make(map[string]externalcontroller.MigrationControllerInfo)
	for modelUUID := range sourceModelUUIDs {
		externalController, err := e.service.ControllerForModel(ctx, modelUUID)
		if err != nil {
			return errors.Trace(err)
		}

		models, err := e.service.ModelsForController(ctx, externalController.ControllerTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		migrationControllerInfo := externalcontroller.MigrationControllerInfo{
			ControllerTag: externalController.ControllerTag,
			Alias:         externalController.Alias,
			Addrs:         externalController.Addrs,
			CACert:        externalController.CACert,
			ModelUUIDs:    models,
		}

		controllers[externalController.ControllerTag.Id()] = migrationControllerInfo
	}

	for _, controller := range controllers {
		_ = model.AddExternalController(description.ExternalControllerArgs{
			Tag:    controller.ControllerTag,
			Addrs:  controller.Addrs,
			Alias:  controller.Alias,
			CACert: controller.CACert,
			Models: controller.ModelUUIDs,
		})
	}

	return nil
}
