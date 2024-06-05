// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/juju/description/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
	logger  logger.Logger
}

// ImportService defines the machine service used to import machines from
// another controller model to this controller.
type ImportService interface {
	CreateMachine(context.Context, string) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import machines"
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ModelDB(), i.logger))
	return nil
}

func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, m := range model.Machines() {
		// We need skeleton machines in dqlite.
		if err := i.service.CreateMachine(ctx, m.Id()); err != nil {
			return fmt.Errorf("importing machine %q: %w", m.Id(), err)
		}
	}
	return nil
}
