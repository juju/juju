// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"sort"
	"strings"

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

	// Get the remote applications so that we can filter out any remote consumer
	// relations, which are not imported as part of the relation domain, but
	// rather as part of the crossmodelrelation domain.
	unique, err := domainmodelmigration.UniqueRemoteOfferApplications(remoteApplications)
	if err != nil {
		return err
	}

	relationRemoteEntities, err := extractRelationUUIDFromRemoteEntities(model)
	if err != nil {
		return errors.Errorf("extracting relation UUIDs from remote entities: %w", err)
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
	if len(args) == 0 {
		return nil
	}

	if err := i.service.ImportRelations(ctx, args); err != nil {
		return errors.Capture(err)
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
	remoteEntities []relationRemoteEntity,
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

type relationRemoteEntity struct {
	RelationKey  corerelation.Key
	RelationUUID string
}

func extractRelationUUIDFromRemoteEntities(model description.Model) ([]relationRemoteEntity, error) {
	var remoteEntities []relationRemoteEntity
	for _, re := range model.RemoteEntities() {
		// Handle only remote entities that are relation UUIDs.
		remoteEntityID := re.ID()
		if !strings.HasPrefix(remoteEntityID, "relation-") {
			continue
		}

		key, err := corerelation.ParseKeyFromTagString(relationTagSuffixToKey(remoteEntityID))
		if err != nil {
			return nil, errors.Errorf("parsing relation key from remote entity id %q: %w", remoteEntityID, err)
		}

		// We shouldn't require the macaroon here, as no connections from the
		// consumer side should be made to the offerer side.
		remoteEntities = append(remoteEntities, relationRemoteEntity{
			RelationKey:  key,
			RelationUUID: re.Token(),
		})
	}
	return remoteEntities, nil
}

func relationTagSuffixToKey(s string) string {
	// Replace both "." with ":" and the "#" with " ".
	s = strings.Replace(s, ".", ":", 2)
	return strings.Replace(s, "#", " ", 1)
}

func findRelationUUID(key corerelation.Key, remoteEntities []relationRemoteEntity) (corerelation.UUID, error) {
	for _, re := range remoteEntities {
		if relationKeysEqual(re.RelationKey, key) {
			return corerelation.UUID(re.RelationUUID), nil
		}
	}
	return corerelation.NewUUID()
}

// relationKeysEqual compares two relation keys for equality, ignoring order.
// Assumes both keys have exactly two endpoints and no scopes.
func relationKeysEqual(a, b corerelation.Key) bool {
	if len(a) != len(b) {
		return false
	}

	// Make defensive copies so that sorting does not mutate the caller's
	// slices.
	aCopy := append(corerelation.Key(nil), a...)
	bCopy := append(corerelation.Key(nil), b...)

	sort.Slice(aCopy, func(i, j int) bool {
		return aCopy[i].String() < aCopy[j].String()
	})
	sort.Slice(bCopy, func(i, j int) bool {
		return bCopy[i].String() < bCopy[j].String()
	})

	// Note: we ignore scope here, as cross model relations do not have scopes
	// when being imported.
	return endpointEquals(aCopy[0], bCopy[0]) && endpointEquals(aCopy[1], bCopy[1])
}

func endpointEquals(a, b corerelation.EndpointIdentifier) bool {
	return a.ApplicationName == b.ApplicationName && a.EndpointName == b.EndpointName
}
