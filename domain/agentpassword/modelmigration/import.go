// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentpassword"
	"github.com/juju/juju/domain/agentpassword/service"
	"github.com/juju/juju/domain/agentpassword/state"
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
	SetUnitPasswordHash(ctx context.Context, unitName coreunit.Name, passwordHash agentpassword.PasswordHash) error
	// SetMachinePasswordHash sets the password hash for the given machine.
	SetMachinePasswordHash(ctx context.Context, machineName coremachine.Name, passwordHash agentpassword.PasswordHash) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import password hashes"
}

// Setup creates the service that is used to import password hashes.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewMigrationService(
		state.NewModelState(scope.ModelDB()),
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

			if err := i.service.SetUnitPasswordHash(ctx, coreunit.Name(unitName), agentpassword.PasswordHash(passwordHash)); err != nil {
				return errors.Errorf("setting password hash for unit %q: %w", unit.Name(), err)
			}
		}
	}

	for _, machine := range model.Machines() {
		passwordHash := machine.PasswordHash()
		if passwordHash == "" {
			continue
		}

		machineName := machine.Id()

		if err := i.service.SetMachinePasswordHash(ctx, coremachine.Name(machineName), agentpassword.PasswordHash(passwordHash)); err != nil {
			return errors.Errorf("setting password hash for machine %q: %w", machineName, err)
		}
	}

	return nil
}
