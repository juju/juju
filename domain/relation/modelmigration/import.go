// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v12"

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

// ImportService provides a subset of the relation domain service methods
// needed for relation import.
type ImportService interface {
	// ImportRelations sets relations imported in migration.
	ImportRelations(ctx context.Context, args relation.ImportRelationsArgs) error

	// ImportRelationData imports the endpoint data, being the application
	// settings, unit settings and unit scope membership, of relations that
	// were created by another domain's import, such as the relations of
	// remote application consumers created by the cross model relation
	// domain.
	ImportRelationData(ctx context.Context, args relation.ImportRelationsArgs) error
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
		i.logger,
	)
	return nil
}

// Execute the import of application resources.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	var (
		remoteApplications = model.RemoteApplications()

		consumerRemoteApplications = domainmodelmigration.GetUniqueRemoteConsumersNames(remoteApplications)
	)

	// Get the remote applications so that we can work out if we need to
	// re-write any remote offerer relation endpoints and keys, to ensure the
	// relations are correctly imported if any remote applications were
	// de-duplicated as part of the import in the cross model relation domain.
	unique, err := domainmodelmigration.UniqueRemoteOfferApplications(remoteApplications)
	if err != nil {
		return err
	}

	relationRemoteEntities, err := domainmodelmigration.
		ExtractRelationUUIDFromRemoteEntities(model)
	if err != nil {
		return errors.Errorf("extracting relation UUIDs from remote entities: %w", err)
	}

	var (
		args     relation.ImportRelationsArgs
		dataArgs relation.ImportRelationsArgs
	)
	for _, rel := range model.Relations() {
		// Relations with remote consumer proxy applications were created by
		// the cross model relation import, which runs first, as the relations
		// are required by the offer connections that are also imported by that
		// domain. Only the relation data is imported here, the relation itself
		// already exists.
		if domainmodelmigration.ContainsRelationEndpointApplicationName(rel, consumerRemoteApplications) {
			arg, err := i.createConsumerProxyImportArg(rel, relationRemoteEntities)
			if err != nil {
				return errors.Errorf("setting up remote consumer relation data for import %d: %w", rel.Id(), err)
			}
			dataArgs = append(dataArgs, arg)
			continue
		}

		// If the relation is a remote offer relation, we need to work out
		// if we need to re-write the relation endpoints, along with the
		// relation key, to ensure that the relation is correctly imported if
		// it has any remote applications that have be de-duplicated as part
		// of the import in cross model relation domain.
		if remoteApps, ok := getRemoteRelation(rel, unique); ok {
			arg, err := i.createRemoteImportArg(rel, remoteApps, relationRemoteEntities)
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
	if len(args) > 0 {
		if err := i.service.ImportRelations(ctx, args); err != nil {
			return errors.Capture(err)
		}
	}

	// If there are no remote consumer relations to import data for, then we
	// can skip calling the service method.
	if len(dataArgs) > 0 {
		if err := i.service.ImportRelationData(ctx, dataArgs); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

func (i *importOperation) createImportArg(rel description.Relation) (relation.ImportRelationArg, error) {
	key, err := corerelation.NewKeyFromString(rel.Key())
	if err != nil {
		return relation.ImportRelationArg{}, err
	}

	uuid, err := corerelation.NewUUID()
	if err != nil {
		return relation.ImportRelationArg{}, err
	}

	arg := relation.ImportRelationArg{
		UUID:  uuid,
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

// createRemoteImportArg creates the import argument for a relation that has
// remote applications, which may have been de-duplicated as part of the cross
// model relation import. This involves re-writing the relation endpoints to use
// the remote application names, and also re-writing the relation key to ensure
// that it is unique across the model if there are multiple relations with
// remote applications that have been de-duplicated to have the same offer UUID.
func (i *importOperation) createRemoteImportArg(
	rel description.Relation,
	remoteApps domainmodelmigration.RemoteApplicationOfferer,
	remoteEntities []domainmodelmigration.RelationRemoteEntity,
) (relation.ImportRelationArg, error) {
	if remoteApps.IsEmpty() {
		// This is a programmatic error, as this function should only be called
		// for relations that have remote applications, so we return an error if
		// there are no remote applications provided.
		return relation.ImportRelationArg{}, errors.New("no remote applications provided for remote relation")
	}

	// The first remote application name is the primary name.
	primaryApplicationName := remoteApps.Primary.Name()

	key, err := corerelation.NewKeyFromString(rel.Key())
	if err != nil {
		return relation.ImportRelationArg{}, err
	}

	// Re-write the relation key to use the remote application names, which are
	// unique across the model, instead of the original application names, which
	// may not be unique if there are multiple remote applications with the same
	// offer UUID that have been de-duplicated.
	for i, ident := range key.EndpointIdentifiers() {
		if ident.ApplicationName == primaryApplicationName {
			continue
		}

		for _, remoteApp := range remoteApps.Duplicates {
			if ident.ApplicationName == remoteApp.Name() {
				ident.ApplicationName = primaryApplicationName
				key[i] = ident
				break
			}
		}
	}

	relationUUID, err := findRelationUUID(key, remoteEntities)
	if err != nil {
		return relation.ImportRelationArg{}, errors.Errorf("finding relation UUID for relation with key %q: %w", key, err)
	}

	arg := relation.ImportRelationArg{
		UUID:  relationUUID,
		ID:    rel.Id(),
		Key:   key,
		Scope: charm.ScopeGlobal,
	}

	for _, v := range rel.Endpoints() {
		if v.Scope() == string(charm.ScopeContainer) {
			arg.Scope = charm.ScopeContainer
		}

		applicationName := v.ApplicationName()
		// Re-write the relation endpoints to use the remote application names,
		// which are unique across the model, instead of the original
		// application names, which may not be unique if there are multiple
		// remote applications with the same offer UUID that have been
		// de-duplicated.
		if applicationName != primaryApplicationName {
			for _, remoteApp := range remoteApps.Duplicates {
				if v.ApplicationName() == remoteApp.Name() {
					applicationName = primaryApplicationName
					break
				}
			}
		}

		arg.Endpoints = append(arg.Endpoints, relation.ImportEndpoint{
			ApplicationName:     applicationName,
			EndpointName:        v.Name(),
			ApplicationSettings: v.ApplicationSettings(),
			UnitSettings:        v.AllSettings(),
		})
	}

	return arg, nil
}

// createConsumerProxyImportArg creates the import argument for the data of a
// relation that has remote consumer proxy applications. The relation itself
// is imported by the cross model relation domain, which requires the relation
// to exist before the relation import operation runs, so that the offer
// connections can reference it. Only the relation data, being the application
// settings, unit settings and unit scope membership, is imported here.
func (i *importOperation) createConsumerProxyImportArg(
	rel description.Relation,
	remoteEntities []domainmodelmigration.RelationRemoteEntity,
) (relation.ImportRelationArg, error) {
	key, err := corerelation.NewKeyFromString(rel.Key())
	if err != nil {
		return relation.ImportRelationArg{}, err
	}

	// The cross model relation domain already created this relation under the
	// UUID of the relation token, so the data is attached to the same UUID.
	relationUUID, err := findConsumerProxyRelationUUID(key, remoteEntities)
	if err != nil {
		return relation.ImportRelationArg{}, errors.Errorf("finding relation UUID for relation with key %q: %w", key, err)
	}

	arg := relation.ImportRelationArg{
		UUID:  relationUUID,
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

func getRemoteRelation(rel description.Relation, remoteApps map[string]domainmodelmigration.RemoteApplicationOfferer) (domainmodelmigration.RemoteApplicationOfferer, bool) {
	for _, endpoint := range rel.Endpoints() {
		appName := endpoint.ApplicationName()
		for _, remoteApp := range remoteApps {
			if remoteApp.Primary.Name() == appName {
				return remoteApp, true
			}

			for _, app := range remoteApp.Duplicates {
				if app.Name() == appName {
					return remoteApp, true
				}
			}
		}
	}
	return domainmodelmigration.RemoteApplicationOfferer{}, false
}

// findRelationUUID returns the relation token recorded in the remote entities
// of the model for the given relation key. The token is the identity that both
// sides of a cross model relation agreed on, so an imported relation keeps
// using it as its relation UUID.
//
// A missing token is not an error: a relation that was never exported to
// another model has no token to reuse, and gets a newly generated UUID.
func findRelationUUID(
	key corerelation.Key,
	remoteEntities []domainmodelmigration.RelationRemoteEntity,
) (corerelation.UUID, error) {
	if token, ok := domainmodelmigration.FindRelationUUID(remoteEntities, key); ok {
		return corerelation.UUID(token), nil
	}
	return corerelation.NewUUID()
}

// findConsumerProxyRelationUUID returns the relation token recorded in the
// remote entities of the model for the given relation key, for a relation of a
// remote application consumer.
//
// Unlike findRelationUUID, a missing token is an error: the cross model
// relation import that created the relation requires the same token, so a miss
// means an inconsistent description, and silently generating a new UUID would
// attach the relation data to a relation that does not exist.
func findConsumerProxyRelationUUID(
	key corerelation.Key,
	remoteEntities []domainmodelmigration.RelationRemoteEntity,
) (corerelation.UUID, error) {
	token, ok := domainmodelmigration.FindRelationUUID(remoteEntities, key)
	if !ok {
		return "", errors.Errorf("no relation UUID found for relation with key %q", key)
	}
	return corerelation.UUID(token), nil
}
