// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/domain/blockcommand/service"
	"github.com/juju/juju/domain/blockcommand/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// ImportService provides a subset of the block command domain
// service methods needed for block command import.
type ImportService interface {
	SwitchBlockOn(ctx context.Context, b blockcommand.BlockType, msg string) error
}

type importOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ImportService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import block commands"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	i.service = service.NewService(
		state.NewState(scope.ModelDB()), i.logger)
	return nil
}

// Execute the import on the block commands contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	blocks := model.Blocks()

	for k, msg := range blocks {
		t, err := importMigrationValue(k)
		if err != nil {
			return errors.Capture(err)
		}

		if err := i.service.SwitchBlockOn(ctx, t, msg); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

func importMigrationValue(t string) (blockcommand.BlockType, error) {
	switch t {
	case "destroy-model":
		return blockcommand.DestroyBlock, nil
	case "remove-object":
		return blockcommand.RemoveBlock, nil
	case "all-changes":
		return blockcommand.ChangeBlock, nil
	default:
		return -1, errors.Errorf("unknown block command type %q", t)
	}
}
