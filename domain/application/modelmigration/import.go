// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/juju/description/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, registry storage.ProviderRegistry, logger logger.Logger) {
	coordinator.Add(&importOperation{
		registry: registry,
		logger:   logger,
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	logger logger.Logger

	service  ImportService
	registry storage.ProviderRegistry
}

// ImportService defines the application service used to import applications
// from another controller model to this controller.
type ImportService interface {
	// CreateApplication registers the existence of an application in the model.
	CreateApplication(context.Context, string, service.AddApplicationParams, ...service.AddUnitParams) error
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB(), i.logger),
		i.logger,
		i.registry,
	)
	return nil
}

func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		unitArgs := make([]service.AddUnitParams, 0, len(app.Units()))
		for _, unit := range app.Units() {
			name := unit.Name()
			unitArgs = append(unitArgs, service.AddUnitParams{UnitName: &name})
		}

		err := i.service.CreateApplication(
			ctx, app.Name(), service.AddApplicationParams{}, unitArgs...,
		)
		if err != nil {
			return fmt.Errorf(
				"import model application %q with %d units: %w",
				app.Name(), len(app.Units()), err,
			)
		}
	}

	return nil
}
