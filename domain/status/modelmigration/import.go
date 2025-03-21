// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/modelmigration"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

func RegisterImport(coordinator Coordinator) {
	coordinator.Add(&importOperation{})
}

type importOperation struct {
	modelmigration.BaseOperation
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import status"
}

// Setup the import operation.
// This will create a new service instance.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	return nil
}

// Execute the import, loading the statuses of the various entities out of the
// description representation, into the domain.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	// TODO: Import statuses here, instead of the application domain
	return nil
}
