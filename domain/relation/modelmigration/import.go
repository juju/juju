// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v11"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corerelation "github.com/juju/juju/core/relation"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment/charm"
	domainmodelmigration "github.com/juju/juju/domain/modelmigration/modelmigration"
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
	unitState := applicationstate.NewInsertIAASUnitState(scope.ModelDB(), i.clock, i.logger)
	i.service = service.NewMigrationService(
		state.NewState(
			scope.ModelDB(),
			i.clock,
			i.logger,
			unitState,
		),
	)
	return nil
}

// Execute the import of application resources.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	// Get the remote applications so that we can filter out any remote consumer
	// relations, which are not imported as part of the relation domain, but
	// rather as part of the crossmodelrelation domain.
	remoteApplications := domainmodelmigration.GetUniqueRemoteConsumersNames(model.RemoteApplications())

	var args relation.ImportRelationsArgs
	for _, rel := range model.Relations() {
		if domainmodelmigration.IsRelationInApplicationsName(rel, remoteApplications) {
			continue
		}

		arg, err := i.createImportArg(rel)
		if err != nil {
			return errors.Errorf("setting up relation data for import %d: %w", rel.Id(), err)
		}
		args = append(args, arg)
	}

	// If there are no relations to import, then we can skip calling the
	// service method.
	if len(args) == 0 {
		return nil
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
		ID:    rel.Id(),
		Key:   key,
		Scope: charm.ScopeGlobal,
	}

	for _, v := range rel.Endpoints() {
		if v.Scope() == string(charm.ScopeContainer) {
			arg.Scope = charm.ScopeContainer
		}
		arg.Endpoints = append(arg.Endpoints, relation.ImportEndpoint{
			ApplicationName:     v.ApplicationName(),
			EndpointName:        v.Name(),
			ApplicationSettings: v.ApplicationSettings(),
			UnitSettings:        v.AllSettings(),
		})
	}
	return arg, nil
}
