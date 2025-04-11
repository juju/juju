// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/sequence/service"
	"github.com/juju/juju/domain/sequence/state"
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

// ImportService defines the sequence service used to import sequences
// from another controller model to this controller.
type ImportService interface {
	// ImportSequences imports the sequences from the given map. This is used to
	// import the sequences from the database.
	ImportSequences(ctx context.Context, seqs map[string]uint64) error

	// RemoveAllSequences removes all sequences from the model. This is used
	// to remove the sequences from the database.
	RemoveAllSequences(ctx context.Context) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import sequences"
}

// Setup creates the service that is used to import sequences.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewMigrationService(
		state.NewState(scope.ModelDB()),
	)
	return nil
}

// Execute the import, adding the sequence to the model. This also includes
// the machines and any units that are associated with the sequence.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	seqs := model.Sequences()
	if len(seqs) == 0 {
		return nil
	}

	s := make(map[string]uint64, len(seqs))
	for k, v := range seqs {
		s[k] = uint64(v)
	}

	return i.service.ImportSequences(ctx, s)
}

// Rollback the import operation. This is required to remove any sequences
// that were added during the import operation.
// For instance, if multiple sequences are add, each with their own
// transaction, then if one fails, the others should be rolled back.
func (i *importOperation) Rollback(ctx context.Context, model description.Model) error {
	return i.service.RemoveAllSequences(ctx)
}
