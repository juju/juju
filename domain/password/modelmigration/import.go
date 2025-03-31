// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/modelmigration"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/password"
	"github.com/juju/juju/domain/password/service"
	"github.com/juju/juju/domain/password/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(
	coordinator Coordinator,
) {
	coordinator.Add(&importOperation{})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
}

// ImportService defines the application service used to import password hashes
// from another controller model to this controller.
type ImportService interface {
	// SetUnitPasswordHash sets the password hash for the given unit.
	SetUnitPasswordHash(ctx context.Context, unitName coreunit.Name, passwordHash password.PasswordHash) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import password hashes"
}

// Setup creates the service that is used to import password hashes.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewMigrationService(
		state.NewState(scope.ModelDB()),
	)
	return nil
}

// Execute the import, adding the password hashes to the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		for _, unit := range app.Units() {
			passwordHash := unit.PasswordHash()
			if passwordHash == "" {
				continue
			}

			unitName := unit.Name()

			if err := i.service.SetUnitPasswordHash(ctx, coreunit.Name(unitName), password.PasswordHash(passwordHash)); err != nil {
				return errors.Errorf("setting password hash for unit %q: %w", unit.Name(), err)
			}
		}
	}

	return nil
}
