// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ModelMigrationState describes persistence methods for migration of cross
// model relations in the model database.
type ModelMigrationState interface {
	// ImportOffers adds offers being migrated to the current model.
	ImportOffers(context.Context, []crossmodelrelation.OfferImport) error

	// ImportRemoteApplicationConsumers adds remote application consumers being
	// migrated to the current model.
	ImportRemoteApplicationConsumers(context.Context, []crossmodelrelation.RemoteApplicationConsumerImport) error

	// ImportRemoteApplicationOfferers adds remote application offerers being
	// migrated to the current model.
	ImportRemoteApplicationOfferers(context.Context, []crossmodelrelation.RemoteApplicationOffererImport) error
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

// RemoteApplicationOffererImport contains details to import a remote
// application offerer during migration. This represents a remote application
// that this model is consuming from another model.
type RemoteApplicationOffererImport struct {
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

	// Units are the unit names for the remote application that need to be
	// created as synthetic units. These are extracted from relation endpoints
	// during migration import.
	Units []string
}

// ImportRemoteApplicationOfferers adds remote application offerers being
// migrated to the current model. These are applications that this model is
// consuming from other models.
func (s *MigrationService) ImportRemoteApplicationOfferers(ctx context.Context, imports []RemoteApplicationOffererImport) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	offerers := make([]crossmodelrelation.RemoteApplicationOffererImport, 0, len(imports))
	for _, rApp := range imports {
		offerer, err := s.constructApplicationOfferer(rApp)
		if err != nil {
			return errors.Errorf("constructing remote application offerer for %q: %w", rApp.Name, err)
		}
		offerers = append(offerers, offerer)
	}

	if err := s.modelState.ImportRemoteApplicationOfferers(ctx, offerers); err != nil {
		return errors.Errorf("importing remote application offerers: %w", err)
	}

	return nil
}

func (s *MigrationService) constructApplicationOfferer(rApp RemoteApplicationOffererImport) (crossmodelrelation.RemoteApplicationOffererImport, error) {
	synthCharm, err := s.constructSyntheticCharm(rApp.Name, rApp.Endpoints)
	if err != nil {
		return crossmodelrelation.RemoteApplicationOffererImport{}, errors.Errorf(
			"constructing synthetic charm for application offerer %q: %w", rApp.Name, err)
	}

	return crossmodelrelation.RemoteApplicationOffererImport{
		Name:            rApp.Name,
		OfferUUID:       rApp.OfferUUID,
		URL:             rApp.URL,
		SourceModelUUID: rApp.SourceModelUUID,
		Macaroon:        rApp.Macaroon,
		Endpoints:       rApp.Endpoints,
		Bindings:        rApp.Bindings,
		Units:           rApp.Units,
		SyntheticCharm:  synthCharm,
	}, nil
}

// RemoteApplicationConsumerImport contains details to import a remote
// application consumer during migration. This represents a remote application
// that this model is offering from another model.
type RemoteApplicationConsumerImport struct {
	// Name is the name of the remote application in this model.
	Name string

	// OfferUUID is the UUID of the offer being consumed.
	OfferUUID string

	// RelationUUID is the UUID of the relation created for this remote
	// application consumer.
	RelationUUID string

	// RelationKey is the key of the relation created for this remote
	// application consumer.
	RelationKey relation.Key

	// URL is the offer URL.
	URL string

	// ConsumerModelUUID is the UUID of the model consuming the application.
	ConsumerModelUUID string

	// ConsumerApplicationUUID is the synthetic application UUID created in the
	// consumer model to represent this remote application.
	ConsumerApplicationUUID string

	// Macaroon is the authentication macaroon for the offer.
	Macaroon string

	// Endpoints are the remote endpoints for creating the synthetic charm.
	// This is kept for backwards compatibility and service layer processing.
	Endpoints []crossmodelrelation.RemoteApplicationEndpoint

	// Bindings are the endpoint-to-space bindings.
	Bindings map[string]string

	// Units are the unit names for the remote application that need to be
	// created as synthetic units. These are extracted from relation endpoints
	// during migration import.
	Units []string

	// Username is the name of the user who made the original offer connection
	// request.
	Username string
}

// ImportRemoteApplicationConsumers adds remote application consumers being
// migrated to the current model. These are applications that this model is
// offering from other models.
func (s *MigrationService) ImportRemoteApplicationConsumers(ctx context.Context, imports []RemoteApplicationConsumerImport) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	consumers := make([]crossmodelrelation.RemoteApplicationConsumerImport, 0, len(imports))
	for _, rApp := range imports {
		consumer, err := s.constructApplicationConsumer(rApp)
		if err != nil {
			return errors.Errorf("constructing remote application consumer for %q: %w", rApp.Name, err)
		}
		consumers = append(consumers, consumer)
	}

	if err := s.modelState.ImportRemoteApplicationConsumers(ctx, consumers); err != nil {
		return errors.Errorf("importing remote application consumers: %w", err)
	}

	return nil
}

func (s *MigrationService) constructApplicationConsumer(rApp RemoteApplicationConsumerImport) (crossmodelrelation.RemoteApplicationConsumerImport, error) {
	appUUID, err := parseRemoteApplicationUUID(rApp.Name)
	if err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, errors.Errorf(
			"parsing remote application UUID: %w", err)
	}

	synthCharm, err := s.constructSyntheticCharm(rApp.Name, rApp.Endpoints)
	if err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, errors.Errorf(
			"constructing synthetic charm: %w", err)
	}

	// We require exactly two entries in the relation key: one for the offering
	// application endpoint, and one for the consuming application endpoint.
	// Peer relations are not supported for cross model relations.
	if len(rApp.RelationKey) != 2 {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, errors.Errorf(
			"invalid relation key length %d: %w", len(rApp.RelationKey), rApp.Name)
	}

	// We can now extract the offering and consuming application endpoints
	// from the relation key.
	var (
		offererApplicationEndpoint  string
		consumerApplicationEndpoint string
	)
	for _, ep := range rApp.RelationKey {
		if ep.ApplicationName == rApp.Name {
			consumerApplicationEndpoint = ep.EndpointName
		} else {
			offererApplicationEndpoint = ep.EndpointName
		}
	}

	return crossmodelrelation.RemoteApplicationConsumerImport{
		Name:                        rApp.Name,
		OfferUUID:                   rApp.OfferUUID,
		URL:                         rApp.URL,
		ConsumerModelUUID:           rApp.ConsumerModelUUID,
		ConsumerApplicationUUID:     rApp.ConsumerApplicationUUID,
		ConsumerApplicationEndpoint: consumerApplicationEndpoint,
		OffererApplicationEndpoint:  offererApplicationEndpoint,
		Macaroon:                    rApp.Macaroon,
		Endpoints:                   rApp.Endpoints,
		Bindings:                    rApp.Bindings,
		Units:                       rApp.Units,

		SyntheticApplicationUUID: appUUID.String(),
		SyntheticCharm:           synthCharm,
	}, nil
}

func (s *MigrationService) constructSyntheticCharm(appName string, endpoints []crossmodelrelation.RemoteApplicationEndpoint) (charm.Charm, error) {
	syntheticCharm, err := constructSyntheticCharm(appName, transform.Slice(endpoints, func(ep crossmodelrelation.RemoteApplicationEndpoint) charm.Relation {
		return charm.Relation{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
		}
	}))
	if err != nil {
		return charm.Charm{}, errors.Errorf("constructing synthetic charm for %q: %w", appName, err)
	}

	// Check that the charm has only one endpoint. There can be multiple
	// synthetic applications per offer, but only one endpoint per synthetic
	// application. To do otherwise requires design and facade changes.
	if err := synthCharmHasOnlyOneEndpoint("????", syntheticCharm); err != nil {
		return charm.Charm{}, internalerrors.Errorf("adding consumed relation: %w", err)
	}

	return syntheticCharm, nil
}

func parseRemoteApplicationUUID(appName string) (coreapplication.UUID, error) {
	if !strings.HasPrefix(appName, "remote-") {
		return "", errors.Errorf(`missing "remote-" prefix`)
	}

	remoteAppUUID, err := uuid.UUIDFromEncodedString(appName[7:])
	if err != nil {
		return "", errors.Errorf("parsing UUID: %w", err)
	}

	return coreapplication.UUID(remoteAppUUID.String()), nil
}
