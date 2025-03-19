// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/domain/blockcommand/service"
	"github.com/juju/juju/domain/blockcommand/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger: logger,
	})
}

// ExportService provides a subset of the block command domain
// service methods needed for block command export.
type ExportService interface {
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

// exportOperation describes a way to execute a migration for
// exporting block devices.
type exportOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export block commands"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	e.service = service.NewService(
		state.NewState(scope.ModelDB()), e.logger)
	return nil
}

// Execute the export, adding the block devices to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	blocks, err := e.service.GetBlocks(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	migration := make(map[string]string)
	for _, block := range blocks {
		migration[exportMigrationValue(block.Type)] = block.Message
	}

	model.SetBlocks(migration)

	return nil
}

func exportMigrationValue(t blockcommand.BlockType) string {
	switch t {
	case blockcommand.DestroyBlock:
		return "destroy-model"
	case blockcommand.RemoveBlock:
		return "remove-object"
	case blockcommand.ChangeBlock:
		return "all-changes"
	default:
		return "unknown"
	}
}
