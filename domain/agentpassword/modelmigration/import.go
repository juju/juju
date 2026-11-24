// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v12"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentpassword"
	"github.com/juju/juju/domain/agentpassword/service"
	"github.com/juju/juju/domain/agentpassword/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

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
		if err := i.importMachinePasswordHash(ctx, machine); err != nil {
			return err
		}
	}

	return nil
}

// importMachinePasswordHash sets the password hash for the machine and for
// every container hosted on it. Containers are nested under their host in
// the description, so they are not returned by model.Machines().
func (i *importOperation) importMachinePasswordHash(ctx context.Context, machine description.Machine) error {
	if passwordHash := machine.PasswordHash(); passwordHash != "" {
		machineName := machine.Id()
		if err := i.service.SetMachinePasswordHash(ctx, coremachine.Name(machineName), agentpassword.PasswordHash(passwordHash)); err != nil {
			return errors.Errorf("setting password hash for machine %q: %w", machineName, err)
		}
	}

	// The description builds each container as a new child of its host, both
	// in AddContainer and when it reads the YAML, so this recursion follows a
	// tree and always ends.
	for _, container := range machine.Containers() {
		if err := i.importMachinePasswordHash(ctx, container); err != nil {
			return err
		}
	}
	return nil
}
