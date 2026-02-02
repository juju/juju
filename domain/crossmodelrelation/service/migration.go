// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/errors"
)

// ModelMigrationState describes persistence methods for migration of cross
// model relations in the model database.
type ModelMigrationState interface {
	// ImportOffers adds offers being migrated to the current model.
	ImportOffers(context.Context, []crossmodelrelation.OfferImport) error

	// ImportRemoteApplicationOfferers adds remote application offerers being migrated
	// to the current model.
	ImportRemoteApplicationOfferers(context.Context, []crossmodelrelation.RemoteApplicationImport) error
}

// MigrationService provides the API for model migration actions within
// the cross model relation domain.
type MigrationService struct {
	modelState ModelMigrationState
	logger     logger.Logger
}

// MigrationService returns a new service reference wrapping the input state
// for migration.
func NewMigrationService(
	modelState ModelMigrationState,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		modelState: modelState,
		logger:     logger,
	}
}

// ImportOffers adds offers being migrated to the current model.
func (s *MigrationService) ImportOffers(ctx context.Context, imports []crossmodelrelation.OfferImport) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return errors.Capture(s.modelState.ImportOffers(ctx, imports))
}

// RemoteApplicationImport contains details to import a remote application
// (offerer) during migration. This represents a remote application that this
// model is consuming from another model.
type RemoteApplicationImport struct {
	// Name is the name of the remote application in this model.
	Name string

	// OfferUUID is the UUID of the offer being consumed.
	OfferUUID string

	// URL is the offer URL.
	URL string

	// SourceModelUUID is the UUID of the model offering the application.
	SourceModelUUID string

	// Macaroon is the authentication macaroon for the offer.
	Macaroon string

	// Endpoints are the remote endpoints for creating the synthetic charm.
	// This is kept for backwards compatibility and service layer processing.
	Endpoints []crossmodelrelation.RemoteApplicationEndpoint

	// Bindings are the endpoint-to-space bindings.
	Bindings map[string]string

	// IsConsumerProxy indicates if this is a consumer proxy (on the offerer
	// side) rather than a remote offerer (on the consumer side).
	IsConsumerProxy bool

	// Units are the unit names for the remote application that need to be
	// created as synthetic units. These are extracted from relation endpoints
	// during migration import.
	Units []string
}

// ImportRemoteApplications adds remote application offerers being migrated to
// the current model. These are applications that this model is consuming from
// other models.
func (s *MigrationService) ImportRemoteApplications(ctx context.Context, imports []RemoteApplicationImport) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Filter out consumer proxies and build synthetic charms for each
	// remote application offerer in the service layer.
	// Consumer proxies represent consumers on the offering side and are
	// handled differently, so they should not be imported as remote offerers.
	offerers := make([]crossmodelrelation.RemoteApplicationImport, 0, len(imports))
	for _, imp := range imports {
		// Skip consumer proxies
		if imp.IsConsumerProxy {
			continue
		}

		offerers = append(offerers, crossmodelrelation.RemoteApplicationImport{
			Name:            imp.Name,
			OfferUUID:       imp.OfferUUID,
			URL:             imp.URL,
			SourceModelUUID: imp.SourceModelUUID,
			Macaroon:        imp.Macaroon,
			Endpoints:       imp.Endpoints,
			Bindings:        imp.Bindings,
			Units:           imp.Units,
			SyntheticCharm:  buildSyntheticCharm(imp.Name, imp.Endpoints),
		})
	}

	return errors.Capture(s.modelState.ImportRemoteApplicationOfferers(ctx, offerers))
}

// buildSyntheticCharm creates a synthetic charm from the remote application's
// endpoints. This is used during migration to recreate the charm that
// represents a remote application.
func buildSyntheticCharm(appName string, endpoints []crossmodelrelation.RemoteApplicationEndpoint) charm.Charm {
	provides := make(map[string]charm.Relation)
	requires := make(map[string]charm.Relation)

	for _, ep := range endpoints {
		rel := charm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Scope:     charm.ScopeGlobal,
		}
		switch ep.Role {
		case charm.RoleProvider:
			provides[ep.Name] = rel
		case charm.RoleRequirer:
			requires[ep.Name] = rel
		}
	}

	return charm.Charm{
		Metadata: charm.Metadata{
			Name:     appName,
			Provides: provides,
			Requires: requires,
		},
		Source:        charm.CMRSource,
		ReferenceName: appName,
	}
}
