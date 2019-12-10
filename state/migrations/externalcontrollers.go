// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description"
	"github.com/juju/errors"
)

// MigrationExternalController represents a state.ExternalController
// Point of use interface to enable better encapsulation.
type MigrationExternalController interface {
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
	AllExternalControllers() ([]MigrationExternalController, error)
}

// ExternalControllerSource composes all the interfaces to create a external
// controllers.
type ExternalControllerSource interface {
	AllExternalControllerSource
}

// ExternalControllerModel defines an in-place usage for adding a
// external controller to a model.
type ExternalControllerModel interface {
	AddExternalController(description.ExternalControllerArgs) description.ExternalController
}

// ExportExternalControllers describes a way to execute a migration for
// exporting external controller s.
type ExportExternalControllers struct{}

// Execute the migration of the offer connections using typed interfaces, to
// ensure we don't loose any type safety.
// This doesn't conform to an interface because go doesn't have generics, but
// when this does arrive this would be an excellent place to use them.
func (m ExportExternalControllers) Execute(src ExternalControllerSource, dst ExternalControllerModel) error {
	externalControllers, err := src.AllExternalControllers()
	if err != nil {
		return errors.Trace(err)
	}

	for _, externalController := range externalControllers {
		if err := m.addExternalController(dst, externalController); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (m ExportExternalControllers) addExternalController(dst ExternalControllerModel, ctrl MigrationExternalController) error {
	_ = dst.AddExternalController(description.ExternalControllerArgs{
		// ID: ctrl.ID(),
		Addrs:  ctrl.Addrs(),
		Alias:  ctrl.Alias(),
		CACert: ctrl.CACert(),
		// Models: ctrl.Models(),
	})
	return nil
}
