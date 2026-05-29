// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v3"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

// MigrationExternalController represents a state.ExternalController
// Point of use interface to enable better encapsulation.
type MigrationExternalController interface {
	// ID holds the controller ID from the external controller
	ID() string

	// Alias holds an alias (human friendly) name for the controller.
	Alias() string

	// Addrs holds the host:port values for the external
	// controller's API server.
	Addrs() []string

	// CACert holds the certificate to validate the external
	// controller's target API server's TLS certificate.
	CACert() string

	// Models holds model UUIDs hosted on this controller.
	Models() []string
}

// AllExternalControllerSource defines an in-place usage for reading all the
// external controllers.
type AllExternalControllerSource interface {
	ControllerForModel(string) (MigrationExternalController, error)

	// ModelExists returns true if the model with the given UUID is
	// hosted on this controller.
	ModelExists(string) (bool, error)

	// LocalControllerInfo returns a MigrationExternalController
	// representing the current controller, with the given model UUIDs
	// as its hosted models. This is used during export to include the
	// local controller as an external controller record so that after
	// migration, the target controller knows how to reach back to the
	// source controller for consumed offers that were local.
	LocalControllerInfo(modelUUIDs []string) (MigrationExternalController, error)
}

// ExternalControllerSource composes all the interfaces to create a external
// controllers.
type ExternalControllerSource interface {
	AllExternalControllerSource
	AllRemoteApplicationSource
}

// ExternalControllerModel defines an in-place usage for adding a
// external controller to a model.
type ExternalControllerModel interface {
	AddExternalController(description.ExternalControllerArgs) description.ExternalController
}

// ExportExternalControllers describes a way to execute a migration for
// exporting external controller s.
type ExportExternalControllers struct{}

// Execute the migration of the external controllers using typed interfaces, to
// ensure we don't loose any type safety.
// This doesn't conform to an interface because go doesn't have generics, but
// when this does arrive this would be an excellent place to use them.
func (m ExportExternalControllers) Execute(src ExternalControllerSource, dst ExternalControllerModel) error {
	// If there are no remote applications, then no external controllers will
	// be exported. We should understand if that's ever going to be an issue?
	remoteApplications, err := src.AllRemoteApplications()
	if err != nil {
		return errors.Trace(err)
	}

	// Iterate over the source model UUIDs, to gather up all the related
	// external controllers. Store them in a map to create a unique set of
	// source model UUIDs, that way we don't request multiple versions of the
	// same external controller.
	sourceModelUUIDs := make(map[string]struct{})
	for _, remoteApp := range remoteApplications {
		sourceModelUUIDs[remoteApp.SourceModel().Id()] = struct{}{}
	}

	controllers := make(map[string]MigrationExternalController)
	var localModelUUIDs []string
	for modelUUID := range sourceModelUUIDs {
		externalController, err := src.ControllerForModel(modelUUID)
		if err != nil {
			if !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			// No external controller record found. Check if the model
			// is hosted on this controller. If so, we need to include
			// the local controller as an external controller in the
			// export, because after migration the consumed offers that
			// were local will be on an external controller.
			exists, existsErr := src.ModelExists(modelUUID)
			if existsErr != nil {
				return errors.Trace(existsErr)
			}
			if !exists {
				return errors.Annotatef(err,
					"cannot find external controller for model %q "+
						"and model is not on this controller", modelUUID)
			}
			localModelUUIDs = append(localModelUUIDs, modelUUID)
			continue
		}
		controllers[externalController.ID()] = externalController
	}

	// If any remote applications reference models on this controller,
	// include the local controller info as an external controller so
	// the target controller can reach back after migration.
	if len(localModelUUIDs) > 0 {
		localCtrl, err := src.LocalControllerInfo(localModelUUIDs)
		if err != nil {
			return errors.Annotate(err,
				"getting local controller info for export")
		}
		controllers[localCtrl.ID()] = localCtrl
	}

	for _, controller := range controllers {
		m.addExternalController(dst, controller)
	}
	return nil
}

func (m ExportExternalControllers) addExternalController(dst ExternalControllerModel, ctrl MigrationExternalController) {
	_ = dst.AddExternalController(description.ExternalControllerArgs{
		Tag:    names.NewControllerTag(ctrl.ID()),
		Addrs:  ctrl.Addrs(),
		Alias:  ctrl.Alias(),
		CACert: ctrl.CACert(),
		Models: ctrl.Models(),
	})
}
