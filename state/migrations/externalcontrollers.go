// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
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
	// If there are not remote applications, then no external controllers will
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
	for modelUUID := range sourceModelUUIDs {
		externalController, err := src.ControllerForModel(modelUUID)
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
		controllers[externalController.ID()] = externalController
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
