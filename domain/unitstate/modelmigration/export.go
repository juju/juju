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

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator) {
	coordinator.Add(&exportOperation{})
}

// ExportService provides a subset of the unitstate domain service methods needed
// for the unitstate export.
type ExportService interface {
	// GetState returns the full unit state. The state may be empty.
	GetState(ctx context.Context, name coreunit.Name) (unitstate.RetrievedUnitState, error)
}

// exportOperation describes a way to execute a migration for exporting the state
// of units.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export unit state"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = unitstateservice.NewService(unitstatestate.NewState(scope.ModelDB()))
	return nil
}

// Execute the export, setting the unit state of every unit of every application
// in the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		for _, unit := range app.Units() {
			unitName, err := coreunit.NewName(unit.Name())
			if err != nil {
				return errors.Errorf("parsing unit name %q: %w", unit.Name(), err)
			}
			state, err := e.service.GetState(ctx, unitName)
			if err != nil {
				return err
			}
			unit.SetCharmState(state.CharmState)
			unit.SetRelationState(state.RelationState)
			unit.SetUniterState(state.UniterState)
			unit.SetStorageState(state.StorageState)
		}
	}
	return nil
}
