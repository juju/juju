// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"sort"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/description/v11"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	modelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	deploymentcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

// ImportService provides a subset of the cross model relation domain
// service methods needed for import.
type ImportService interface {
	// ImportOffers adds offers being migrated to the current model.
	ImportOffers(context.Context, []crossmodelrelation.OfferImport) error

	// ImportRemoteApplicationOfferers adds remote application offerers being
	// migrated to the current model.
	ImportRemoteApplicationOfferers(context.Context, []service.RemoteApplicationOffererImport) error

	// ImportRemoteApplicationConsumers adds remote application consumers being
	// migrated to the current model.
	ImportRemoteApplicationConsumers(context.Context, []service.RemoteApplicationConsumerImport) error
}

type importOperation struct {
	modelmigration.BaseOperation

	importService ImportService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import cross model relations"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.importService = service.NewMigrationService(
		modelstate.NewState(scope.ModelDB(), "", i.clock, i.logger),
		i.logger,
	)
	return nil
}

// Execute the import of the cross model relations contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	if err := i.importOffers(ctx, model.Applications()); err != nil {
		return errors.Errorf("importing offers: %w", err)
	}

	remoteApplications := model.RemoteApplications()
	if len(remoteApplications) == 0 {
		return nil
	}

	// Extract unit names for remote applications from relations.
	// This is needed because synthetic units must be created before
	// relations can be imported.
	remoteAppUnits := i.extractRemoteAppUnits(model)

	// Import remote application offerers, this will create the synthetic
	// applications and units needed for relations.
	if err := i.importRemoteApplicationOfferers(ctx, remoteApplications, remoteAppUnits); err != nil {
		return errors.Errorf("importing remote applications: %w", err)
	}

	// Extract offer connections for remote application consumers. We'll
	// need these to use the correct user when importing the consumers.
	offerConnections, err := extractOfferConnections(model)
	if err != nil {
		return errors.Errorf("extracting offer connections: %w", err)
	}

	// Extract remote entities for application and relation UUIDs, or commonly
	// called tokens in prior Juju versions.
	relationRemoteEntities, err := extractRelationUUIDFromRemoteEntities(model)
	if err != nil {
		return errors.Errorf("extracting relation UUIDs from remote entities: %w", err)
	}
	applicationRemoteEntities, err := extractApplicationUUIDFromRemoteEntities(model)
	if err != nil {
		return errors.Errorf("extracting application UUIDs from remote entities: %w", err)
	}
	relationEndpoints, err := extractRelationEndpoints(model)
	if err != nil {
		return errors.Errorf("extracting relation endpoints from relations: %w", err)
	}

	// Import remote application consumers.
	if err := i.importRemoteApplicationConsumers(ctx,
		remoteApplications,
		remoteAppUnits,
		offerConnections,
		relationRemoteEntities,
		relationEndpoints,
		applicationRemoteEntities,
	); err != nil {
		return errors.Errorf("importing remote application consumers: %w", err)
	}

	return nil
}

// extractRemoteAppUnits scans relations to find unit names for each remote
// application. Unit names are extracted from relation endpoint settings.
func (i *importOperation) extractRemoteAppUnits(model description.Model) map[string][]string {
	// Build set of remote application names.
	remoteAppNames := make(map[string]struct{})
	for _, ra := range model.RemoteApplications() {
		remoteAppNames[ra.Name()] = struct{}{}
	}

	// Extract unit names from relation endpoints.
	remoteAppUnits := make(map[string]map[string]struct{})
	for _, rel := range model.Relations() {
		for _, ep := range rel.Endpoints() {
			appName := ep.ApplicationName()
			if _, isRemote := remoteAppNames[appName]; !isRemote {
				continue
			}

			if remoteAppUnits[appName] == nil {
				remoteAppUnits[appName] = make(map[string]struct{})
			}

			// Unit names are the keys of AllSettings().
			for unitName := range ep.AllSettings() {
				remoteAppUnits[appName][unitName] = struct{}{}
			}
		}
	}

	// Convert sets to slices.
	result := make(map[string][]string)
	for appName, unitSet := range remoteAppUnits {
		units := make([]string, 0, len(unitSet))
		for unitName := range unitSet {
			units = append(units, unitName)
		}
		result[appName] = units
	}
	return result
}

func (i *importOperation) importOffers(ctx context.Context, apps []description.Application) error {
	input := make([]crossmodelrelation.OfferImport, 0)
	for _, app := range apps {
		for _, offer := range app.Offers() {
			offerUUID, err := uuid.UUIDFromString(offer.OfferUUID())
			if err != nil {
				return errors.Errorf("validating uuid for offer %q,%q: %w",
					offer.ApplicationName(), offer.OfferName(), err)
			}

			endpoints := transform.MapToSlice(
				offer.Endpoints(),
				func(k, v string) []string {
					return []string{v}
				},
			)
			imp := crossmodelrelation.OfferImport{
				UUID:            offerUUID,
				Name:            offer.OfferName(),
				ApplicationName: offer.ApplicationName(),
				Endpoints:       endpoints,
			}
			input = append(input, imp)
		}
	}
	if len(input) == 0 {
		return nil
	}
	return i.importService.ImportOffers(ctx, input)
}

func (i *importOperation) importRemoteApplicationOfferers(
	ctx context.Context,
	remoteApps []description.RemoteApplication,
	remoteAppUnits map[string][]string,
) error {
	input := make([]service.RemoteApplicationOffererImport, 0, len(remoteApps))
	for _, remoteApp := range remoteApps {
		// Ignore remote application consumers.
		if remoteApp.IsConsumerProxy() {
			continue
		}

		endpoints, err := extractRemoteEndpoints(remoteApp)
		if err != nil {
			return errors.Errorf("extracting endpoints for remote application %q: %w",
				remoteApp.Name(), err)
		}

		input = append(input, service.RemoteApplicationOffererImport{
			RemoteApplicationImport: service.RemoteApplicationImport{
				Name:            remoteApp.Name(),
				OfferUUID:       remoteApp.OfferUUID(),
				URL:             remoteApp.URL(),
				SourceModelUUID: remoteApp.SourceModelUUID(),
				Macaroon:        remoteApp.Macaroon(),
				Endpoints:       endpoints,
				Bindings:        remoteApp.Bindings(),
				Units:           remoteAppUnits[remoteApp.Name()],
			},
		})
	}
	if len(input) == 0 {
		return nil
	}
	return i.importService.ImportRemoteApplicationOfferers(ctx, input)
}

func (i *importOperation) importRemoteApplicationConsumers(
	ctx context.Context,
	remoteApps []description.RemoteApplication,
	remoteAppUnits map[string][]string,
	offerConnections []offerConnection,
	relationRemoteEntities []relationRemoteEntity,
	relationEndpoints map[string]relation.Key,
	applicationRemoteEntities map[string]string,
) error {
	input := make([]service.RemoteApplicationConsumerImport, 0, len(remoteApps))
	for _, remoteApp := range remoteApps {
		// Ignore remote application offerers.
		if !remoteApp.IsConsumerProxy() {
			continue
		}

		endpoints, err := extractRemoteEndpoints(remoteApp)
		if err != nil {
			return errors.Errorf("extracting endpoints for remote application %q: %w",
				remoteApp.Name(), err)
		}

		// Note: we can't use the remoteApp.OfferUUID as it's not filled in for
		// consumers, only offerers. This means that we have to go hunting for
		// it in the offer connections.

		// Extract the username from the offer connection, this should tell us
		// who made the original offer connection request in the source model.
		offerConnection, err := findOfferConnection(offerConnections, remoteApp.Name(), remoteApp.SourceModelUUID())
		if err != nil {
			return errors.Errorf("extracting offer connection user name for remote application %q: %w",
				remoteApp.Name(), err)
		}

		relationUUID, err := findRelationUUIDForKey(relationRemoteEntities, offerConnection.RelationKey)
		if err != nil {
			return errors.Errorf("finding relation UUID for remote application %q: %w",
				remoteApp.Name(), err)
		}

		consumerApplicationUUID, ok := applicationRemoteEntities[remoteApp.Name()]
		if !ok {
			return errors.Errorf("no consumer application UUID found for remote application %q",
				remoteApp.Name())
		}

		relationKey, ok := relationEndpoints[offerConnection.RelationKeyStr]
		if !ok {
			return errors.Errorf("no relation endpoints found for relation key %q for remote application %q",
				offerConnection.RelationKeyStr, remoteApp.Name())
		}

		input = append(input, service.RemoteApplicationConsumerImport{
			RemoteApplicationImport: service.RemoteApplicationImport{
				Name:      remoteApp.Name(),
				OfferUUID: offerConnection.OfferUUID,
				URL:       remoteApp.URL(),
				Macaroon:  remoteApp.Macaroon(),
				Endpoints: endpoints,
				Bindings:  remoteApp.Bindings(),
				Units:     remoteAppUnits[remoteApp.Name()],
			},
			RelationUUID:            relationUUID,
			RelationKey:             relationKey,
			ConsumerModelUUID:       remoteApp.SourceModelUUID(),
			ConsumerApplicationUUID: consumerApplicationUUID,
			UserName:                offerConnection.UserName,
		})
	}
	if len(input) == 0 {
		return nil
	}
	return i.importService.ImportRemoteApplicationConsumers(ctx, input)
}

type offerConnection struct {
	OfferUUID       string
	RelationKey     relation.Key
	RelationKeyStr  string
	SourceModelUUID string
	UserName        string
}

func extractOfferConnections(model description.Model) ([]offerConnection, error) {
	var offerConnections []offerConnection
	for _, rel := range model.OfferConnections() {
		offerUUID := rel.OfferUUID()

		relationKey, err := relation.NewKeyFromString(rel.RelationKey())
		if err != nil {
			return nil, errors.Errorf("parsing relation key %q: %w", rel.RelationKey(), err)
		}

		offerConnections = append(offerConnections, offerConnection{
			OfferUUID:       offerUUID,
			RelationKey:     relationKey,
			RelationKeyStr:  rel.RelationKey(),
			SourceModelUUID: rel.SourceModelUUID(),
			UserName:        rel.UserName(),
		})
	}
	return offerConnections, nil
}

type relationRemoteEntity struct {
	RelationKey  relation.Key
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

		key, err := relation.ParseKeyFromTagString(relationTagSuffixToKey(remoteEntityID))
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

func extractApplicationUUIDFromRemoteEntities(model description.Model) (map[string]string, error) {
	remoteEntities := make(map[string]string)
	for _, re := range model.RemoteEntities() {
		// Handle only remote entities that are application UUIDs.
		remoteEntityID := re.ID()
		if !strings.HasPrefix(remoteEntityID, "application-") {
			continue
		}

		tag, err := names.ParseApplicationTag(remoteEntityID)
		if err != nil {
			return nil, errors.Errorf("parsing application tag from remote entity id %q: %w", remoteEntityID, err)
		}

		remoteEntities[tag.Id()] = re.Token()
	}
	return remoteEntities, nil
}

func extractRemoteEndpoints(remoteApp description.RemoteApplication) ([]crossmodelrelation.RemoteApplicationEndpoint, error) {
	endpoints := make([]crossmodelrelation.RemoteApplicationEndpoint, 0, len(remoteApp.Endpoints()))
	for _, ep := range remoteApp.Endpoints() {
		role, err := parseRelationRole(ep.Role())
		if err != nil {
			return nil, errors.Errorf("parsing role for endpoint %q: %w",
				ep.Name(), err)
		}
		endpoints = append(endpoints, crossmodelrelation.RemoteApplicationEndpoint{
			Name:      ep.Name(),
			Role:      role,
			Interface: ep.Interface(),
		})
	}
	return endpoints, nil
}

func extractRelationEndpoints(model description.Model) (map[string]relation.Key, error) {
	relationEndpoints := make(map[string]relation.Key)
	for _, rel := range model.Relations() {
		var key relation.Key
		for _, ep := range rel.Endpoints() {
			role, err := parseRelationRole(ep.Role())
			if err != nil {
				return nil, errors.Errorf("parsing role for relation endpoint: %w", err)
			}

			key = append(key, relation.EndpointIdentifier{
				ApplicationName: ep.ApplicationName(),
				EndpointName:    ep.Name(),
				Role:            deploymentcharm.RelationRole(role),
			})
		}

		relationEndpoints[rel.Key()] = key
	}
	return relationEndpoints, nil
}

func findOfferConnection(offerConns []offerConnection, appName, modelUUID string) (offerConnection, error) {
	if len(offerConns) == 0 {
		return offerConnection{}, errors.Errorf("no offer connections for application %q", appName)
	}

	for _, conn := range offerConns {
		if conn.SourceModelUUID != modelUUID {
			continue
		}

		for _, key := range conn.RelationKey {
			if key.ApplicationName == appName {
				return conn, nil
			}
		}
	}

	return offerConnection{}, errors.Errorf("no offer connection contains application %q", appName)
}

func findRelationUUIDForKey(remoteEntities []relationRemoteEntity, relationKey relation.Key) (string, error) {
	if len(relationKey) != 2 {
		return "", errors.Errorf("expected relation key with 2 endpoints, got %d", len(relationKey))
	}

	for _, remoteEntity := range remoteEntities {
		key := remoteEntity.RelationKey
		if relationKeysEqual(key, relationKey) {
			return remoteEntity.RelationUUID, nil
		}
	}

	return "", errors.Errorf("no relation UUID found for relation key %q", relationKey.String())
}

func relationTagSuffixToKey(s string) string {
	// Replace both "." with ":" and the "#" with " ".
	s = strings.Replace(s, ".", ":", 2)
	return strings.Replace(s, "#", " ", 1)
}

// relationKeysEqual compares two relation keys for equality, ignoring order.
// Assumes both keys have exactly two endpoints and no scopes.
func relationKeysEqual(a, b relation.Key) bool {
	if len(a) != len(b) {
		return false
	}

	// Make defensive copies so that sorting does not mutate the caller's
	// slices.
	aCopy := append(relation.Key(nil), a...)
	bCopy := append(relation.Key(nil), b...)

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

func endpointEquals(a, b relation.EndpointIdentifier) bool {
	return a.ApplicationName == b.ApplicationName && a.EndpointName == b.EndpointName
}

// parseRelationRole parses a string role to a charm.RelationRole.
func parseRelationRole(role string) (charm.RelationRole, error) {
	switch role {
	case "provider":
		return charm.RoleProvider, nil
	case "requirer":
		return charm.RoleRequirer, nil
	case "peer":
		return charm.RolePeer, nil
	default:
		return "", errors.Errorf("unknown relation role %q", role)
	}
}
