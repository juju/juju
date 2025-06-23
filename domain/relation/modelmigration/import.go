// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/relation/service"
	"github.com/juju/juju/domain/relation/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(
	coordinator Coordinator,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

// ImportService provides a subset of the resource domain service methods
// needed for resource import.
type ImportService interface {
	// ImportRelations sets relations imported in migration.
	ImportRelations(ctx context.Context, args relation.ImportRelationsArgs) error

	// DeleteImportedRelations deletes all imported relations in a model during
	// an import rollback.
	DeleteImportedRelations(
		ctx context.Context,
	) error
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import relations"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(
			scope.ModelDB(),
			i.clock,
			i.logger,
		),
		i.logger)
	return nil
}

// Execute the import of application resources.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	var args relation.ImportRelationsArgs
	relations := model.Relations()
	if len(relations) == 0 {
		return nil
	}
	for _, rel := range relations {
		arg, err := i.createImportArg(rel)
		if err != nil {
			return errors.Errorf("setting up relation data for import %d: %w", rel.Id(), err)
		}
		args = append(args, arg)
	}
	err := i.service.ImportRelations(ctx, args)
	if err != nil {
		return errors.Errorf("importing relations: %w", err)
	}
	return nil
}

func (i *importOperation) createImportArg(rel description.Relation) (relation.ImportRelationArg, error) {
	key, err := corerelation.NewKeyFromString(rel.Key())
	if err != nil {
		return relation.ImportRelationArg{}, err
	}

	arg := relation.ImportRelationArg{
		Endpoints: []relation.ImportEndpoint{},
		ID:        rel.Id(),
		Key:       key,
	}
	for _, v := range rel.Endpoints() {
		endpoint := relation.ImportEndpoint{
			ApplicationName:     v.ApplicationName(),
			EndpointName:        v.Name(),
			ApplicationSettings: v.ApplicationSettings(),
			UnitSettings:        v.AllSettings(),
		}
		arg.Endpoints = append(arg.Endpoints, endpoint)
	}
	return arg, nil
}

// Rollback the resource import operation by deleting all imported resources
// associated with the imported applications.
func (i *importOperation) Rollback(ctx context.Context, model description.Model) error {
	if len(model.Relations()) == 0 {
		return nil
	}
	err := i.service.DeleteImportedRelations(ctx)
	if err != nil {
		return errors.Errorf("resource import rollback failed: %w", err)
	}
	return nil
}
