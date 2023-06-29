// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/description/v4"
	"github.com/juju/errors"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/externalcontroller"
)

type ExportState interface {
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)
	ModelsForController(ctx context.Context, controllerUUID string) ([]string, error)
}

// ExportOperation describes a way to execute a migration for
// exporting external controller s.
type ExportOperation struct {
	st      ExportState
	stateFn func(database.TxnRunner) (ExportState, error)
}

func (e *ExportOperation) Setup(dbGetter database.DBGetter) error {
	db, err := dbGetter.GetDB(database.ControllerNS)
	if err != nil {
		return errors.Annotatef(err, "retrieving database for export operation")
	}

	e.st, err = e.stateFn(db)
	if err != nil {
		return errors.Annotatef(err, "retrieving state for export operation")
	}

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
func (e *ExportOperation) Execute(ctx context.Context, dst description.Model) error {
	// If there are not remote applications, then no external controllers will
	// be exported. We should understand if that's ever going to be an issue?
	remoteApplications := dst.RemoteApplications()

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
		externalController, err := e.st.ControllerForModel(ctx, modelUUID)
		if err != nil {
			// This can occur when attempting to export a remote application
			// where there is a external controller, yet the controller doesn't
			// exist.
			// This generally only happens whilst keeping backwards
			// compatibility, whilst remote applications aren't exported or
			// imported correctly.
			// TODO (stickupkid): This should be removed when we support CMR
			// migrations without a feature flag.
			if errors.IsNotFound(err) {
				continue
			}
			return errors.Trace(err)
		}

		models, err := e.st.ModelsForController(ctx, externalController.ControllerTag.Id())
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
		_ = dst.AddExternalController(description.ExternalControllerArgs{
			Tag:    controller.ControllerTag,
			Addrs:  controller.Addrs,
			Alias:  controller.Alias,
			CACert: controller.CACert,
			Models: controller.ModelUUIDs,
		})
	}

	return nil
}
