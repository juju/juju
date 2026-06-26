// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/description/v12"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/internal/errors"
)

const IntentionalImportFailure = errors.ConstError("intentional import failure")

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

func RegisterFailingImport(coordinator Coordinator) {
	coordinator.Add(&failingImportOperation{})
}

type failingImportOperation struct {
	modelmigration.BaseOperation
}

func (i *failingImportOperation) Setup(_ modelmigration.Scope) error {
	return nil
}

func (i *failingImportOperation) Execute(_ context.Context, _ description.Model) error {
	return IntentionalImportFailure
}

func (i *failingImportOperation) Name() string {
	return "failing-import-operation"
}
