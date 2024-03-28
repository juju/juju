// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/juju/description/v5"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
)

var logger = loggo.GetLogger("juju.migration.machines")

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator) {
	coordinator.Add(&importOperation{})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
}

// ImportService defines the machine service used to import machines from
// another controller model to this controller.
type ImportService interface {
	CreateMachine(context.Context, string) error
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ModelDB(), logger))
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
