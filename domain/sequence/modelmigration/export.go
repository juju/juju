// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/sequence/service"
	"github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(
	coordinator Coordinator,
) {
	coordinator.Add(&exportOperation{})
}

// ExportService provides a subset of the sequence domain
// service methods needed for sequence export.
type ExportService interface {
	// GetSequencesForExport returns the sequences for export. This is used to
	// retrieve the sequences for export in the database.
	GetSequencesForExport(ctx context.Context) (map[string]uint64, error)
}

// exportOperation describes a way to execute a migration for
// exporting sequences.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export sequences"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewMigrationService(
		state.NewState(scope.ModelDB()),
	)
	return nil
}

// Execute the export, adding the sequence to the model.
// The export also includes all the charm metadata, manifest, config and
// actions. Along with units and resources.
func (e *exportOperation) Execute(ctx context.Context, m description.Model) error {
	seqs, err := e.service.GetSequencesForExport(ctx)
	if err != nil {
		return errors.Errorf("getting sequences for export: %w", err)
	}

	for name, value := range seqs {
		m.SetSequence(name, int(value))
	}
	return nil
}
