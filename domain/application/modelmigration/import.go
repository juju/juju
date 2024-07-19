// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/juju/description/v8"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	internalcharm "github.com/juju/juju/internal/charm"
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
	CreateApplication(context.Context, string, internalcharm.Charm, service.AddApplicationArgs, ...service.AddUnitArg) (coreapplication.ID, error)
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import applications"
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB(), i.logger),
		i.registry,
		i.logger,
	)
	return nil
}

func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		unitArgs := make([]service.AddUnitArg, 0, len(app.Units()))
		for _, unit := range app.Units() {
			name := unit.Name()
			unitArgs = append(unitArgs, service.AddUnitArg{UnitName: &name})
		}

		_, err := i.service.CreateApplication(
			ctx, app.Name(), &stubCharm{}, service.AddApplicationArgs{}, unitArgs...,
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

type stubCharm struct {
	internalcharm.Charm
}

func (s stubCharm) Meta() *internalcharm.Meta {
	return &internalcharm.Meta{
		Name: "stub",
	}
}
