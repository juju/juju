// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/relation/service"
	"github.com/juju/juju/domain/relation/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		clock:  clock,
		logger: logger,
	})
}

// ExportService provides a subset of the relation domain
// service methods needed for relation export.
type ExportService interface {
	ExportRelations(ctx context.Context) ([]relation.ExportRelation, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	exportService ExportService
	logger        logger.Logger
	clock         clock.Clock
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export relations"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.exportService = service.NewService(
		state.NewState(scope.ModelDB(), e.clock, e.logger),
		e.logger,
	)
	return nil
}

// Execute the migration export, which adds the relations to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	relations, err := e.exportService.ExportRelations(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	for _, relation := range relations {
		rel := model.AddRelation(description.RelationArgs{
			Id:  relation.ID,
			Key: relation.Key.String(),
		})

		for _, endpoint := range relation.Endpoints {
			ep := rel.AddEndpoint(description.EndpointArgs{
				ApplicationName: endpoint.ApplicationName,
				Name:            endpoint.Name,
				Role:            string(endpoint.Role),
				Interface:       endpoint.Interface,
				Optional:        endpoint.Optional,
				Limit:           endpoint.Limit,
				Scope:           string(endpoint.Scope),
			})

			ep.SetApplicationSettings(endpoint.ApplicationSettings)
			for unitName, unitSettings := range endpoint.AllUnitSettings {
				ep.SetUnitSettings(unitName, unitSettings)
			}
		}
	}

	return nil
}
