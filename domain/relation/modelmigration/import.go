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
	var (
		relations          = model.Relations()
		remoteApplications = model.RemoteApplications()

		consumerRemoteApplications = domainmodelmigration.GetUniqueRemoteConsumersNames(remoteApplications)
	)

	// Get the remote applications so that we can filter out any remote consumer
	// relations, which are not imported as part of the relation domain, but
	// rather as part of the crossmodelrelation domain.
	unique, err := domainmodelmigration.UniqueRemoteOfferApplications(remoteApplications, relations)
	if err != nil {
		return err
	}

	var args relation.ImportRelationsArgs
	for _, rel := range model.Relations() {
		// If the relation is a remote consumer relation we skip it.
		if domainmodelmigration.ContainsRelationEndpointApplicationName(rel, consumerRemoteApplications) {
			continue
		}

		// If the relation is a remote offer relation, we need to work out
		// if we need to re-write the relation endpoints, along with the
		// relation key, to ensure that the relation is correctly imported if
		// it has any remote applications that have be de-duplicated as part of
		// the import in cross model relation domain.
		if remoteApp, ok := matchesRemoteAppEndpoint(rel, unique); ok {
			arg, err := i.createRemoteImportArg(rel, remoteApp)
			if err != nil {
				return errors.Errorf("setting up remote relation data for import %d: %w", rel.Id(), err)
			}
			args = append(args, arg)
			continue
		}

		// This is a standard relation that we can import as is.
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

	if err := i.service.ImportRelations(ctx, args); err != nil {
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

func (i *importOperation) createRemoteImportArg(rel description.Relation, remoteApp description.RemoteApplication) (relation.ImportRelationArg, error) {

}

func matchesRemoteAppEndpoint(rel description.Relation, remoteApps map[string]description.RemoteApplication) (description.RemoteApplication, bool) {
	for _, endpoint := range rel.Endpoints() {
		appName := endpoint.ApplicationName()
		for _, remoteApp := range remoteApps {
			if remoteApp.Name() == appName {
				return remoteApp, true
			}
		}
	}
	return nil, false
}
