// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/description/v11"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	modelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
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
	offerConnections := i.extractOfferConnections(model)

	// Import remote application consumers.
	if err := i.importRemoteApplicationConsumers(ctx, remoteApplications, remoteAppUnits, offerConnections); err != nil {
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

type OfferConnection struct {
	RelationKey     string
	SourceModelUUID string
	UserName        string
}

func (i *importOperation) extractOfferConnections(model description.Model) map[string][]OfferConnection {
	offerConnections := make(map[string][]OfferConnection)
	for _, rel := range model.OfferConnections() {
		offerUUID := rel.OfferUUID()

		offerConnections[offerUUID] = append(offerConnections[offerUUID], OfferConnection{
			RelationKey:     rel.RelationKey(),
			SourceModelUUID: rel.SourceModelUUID(),
			UserName:        rel.UserName(),
		})
	}
	return offerConnections
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
		endpoints := make([]crossmodelrelation.RemoteApplicationEndpoint, 0, len(remoteApp.Endpoints()))
		for _, ep := range remoteApp.Endpoints() {
			role, err := parseRelationRole(ep.Role())
			if err != nil {
				return errors.Errorf("parsing role for endpoint %q on remote app %q: %w",
					ep.Name(), remoteApp.Name(), err)
			}
			endpoints = append(endpoints, crossmodelrelation.RemoteApplicationEndpoint{
				Name:      ep.Name(),
				Role:      role,
				Interface: ep.Interface(),
			})
		}

		offerUUID := remoteApp.OfferUUID()

		input = append(input, service.RemoteApplicationOffererImport{
			Name:            remoteApp.Name(),
			OfferUUID:       offerUUID,
			URL:             remoteApp.URL(),
			SourceModelUUID: remoteApp.SourceModelUUID(),
			Macaroon:        remoteApp.Macaroon(),
			Endpoints:       endpoints,
			Bindings:        remoteApp.Bindings(),
			Units:           remoteAppUnits[remoteApp.Name()],
		})
	}
	return i.importService.ImportRemoteApplicationOfferers(ctx, input)
}

func (i *importOperation) importRemoteApplicationConsumers(
	ctx context.Context,
	remoteApps []description.RemoteApplication,
	remoteAppUnits map[string][]string,
	offerConnections map[string][]OfferConnection,
) error {
	input := make([]service.RemoteApplicationConsumerImport, 0, len(remoteApps))
	for _, remoteApp := range remoteApps {
		offerUUID := remoteApp.OfferUUID()

		conns, ok := offerConnections[offerUUID]
		if !ok {
			return errors.Errorf("no offer connections found for remote application %q with offer uuid %q",
				remoteApp.Name(), remoteApp.OfferUUID())
		}

		// Extract the username from the offer connection, this should tell
		// us who made the original offer connection request in the source
		// model.
		username, err := extractOfferConnectionUsername(remoteApp.Name(), conns)
		if err != nil {
			return errors.Errorf("extracting offer connection user name for remote application %q: %w",
				remoteApp.Name(), err)
		}

		input = append(input, service.RemoteApplicationConsumerImport{
			Name:            remoteApp.Name(),
			OfferUUID:       offerUUID,
			RelationUUID:    remoteApp.RelationUUID(),
			URL:             remoteApp.URL(),
			SourceModelUUID: remoteApp.SourceModelUUID(),
			Macaroon:        remoteApp.Macaroon(),
			Endpoints:       remoteApp.Endpoints(),
			Bindings:        remoteApp.Bindings(),
			Units:           remoteAppUnits[remoteApp.Name()],
			Username:        username,
		})
	}
	return i.importService.ImportRemoteApplicationConsumers(ctx, input)
}

func extractOfferConnectionUsername(appName string, conns []OfferConnection) (string, error) {
	if len(conns) == 0 {
		return "", errors.Errorf("no offer connections for application %q", appName)
	}

	for _, conn := range conns {
		parts, err := relation.NewKeyFromString(conn.RelationKey)
		if err != nil {
			return "", errors.Errorf("parsing relation key %q: %w", conn.RelationKey, err)
		}

		for _, part := range parts {
			if part.ApplicationName == appName {
				return conn.UserName, nil
			}
		}
	}

	return "", errors.Errorf("no offer connection contains application %q", appName)
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
