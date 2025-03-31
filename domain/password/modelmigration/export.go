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

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(
	coordinator Coordinator,
) {
	coordinator.Add(&exportOperation{})
}

// ExportService is the interface that provides the methods for exporting
// password hashes.
type ExportService interface {
	// GetAllUnitPasswordHashes returns a map of unit names to password hashes.
	GetAllUnitPasswordHashes(context.Context) (map[string]map[coreunit.Name]password.PasswordHash, error)
}

// exportOperation describes a way to execute a migration for
// exporting applications.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export password hashes"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewMigrationService(
		state.NewState(scope.ModelDB()),
	)
	return nil
}

// Execute the export, adding the application to the model.
// The export also includes all the charm metadata, manifest, config and
// actions. Along with units and resources.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	passwords, err := e.service.GetAllUnitPasswordHashes(ctx)
	if err != nil {
		return errors.Errorf("getting all unit password hashes: %w", err)
	}

	for _, app := range model.Applications() {
		units, ok := passwords[app.Name()]
		if !ok {
			continue
		}

		for _, unit := range app.Units() {
			unitName := coreunit.Name(unit.Name())

			password, ok := units[unitName]
			if !ok {
				continue
			}
			unit.SetPasswordHash(password.String())
		}
	}
	return nil
}
