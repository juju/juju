// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	"github.com/juju/juju/core/modelmigration"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate"
	unitstateservice "github.com/juju/juju/domain/unitstate/service"
	unitstatestate "github.com/juju/juju/domain/unitstate/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport registers a new model migration importer to the coordinator,
// for importing unit states.
func RegisterImport(coordinator Coordinator) {
	coordinator.Add(&importOperation{})
}

// ImportService provides a subset of the unitstate domain service methods needed
// for the unitstate import.
type ImportService interface {
	// SetState persists the input unit state selectively,
	// based on its populated values.
	SetState(context.Context, unitstate.UnitState) error
}

// importOperation describes a way to execute a migration for importing the state
// of units.
type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import unit state"
}

// Setup the import operation.
// This will create a new service instance.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = unitstateservice.NewService(unitstatestate.NewState(scope.ModelDB()))
	return nil
}

// Execute the import, setting the unit states for all the units for all the applications
// in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		for _, unit := range app.Units() {
			unitName, err := coreunit.NewName(unit.Name())
			if err != nil {
				return err
			}
			charmState := unit.CharmState()
			relationState := unit.RelationState()
			uniterState := unit.UniterState()
			storageState := unit.StorageState()
			args := unitstate.UnitState{
				Name:         unitName,
				UniterState:  &uniterState,
				StorageState: &storageState,
			}
			// These next two are optional, simply because they can take a nil
			// value, and the others can't.
			if charmState != nil {
				args.CharmState = &charmState
			}
			if relationState != nil {
				args.RelationState = &relationState
			}
			if err := i.service.SetState(ctx, args); err != nil {
				return errors.Errorf("setting unit state for %q: %w", unitName, err)
			}
		}
	}
	return nil
}
