// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
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

	// GetApplicationUUIDByName returns nil if the application exists.
	// Otherwise, it returns an error.
	GetApplicationUUIDByName(ctx context.Context, name string) (string, error)
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

	return internalerrors.Capture(s.modelState.ImportOffers(ctx, imports))
}

// RemoteApplicationImport contains details to import a remote application
// during migration.
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

	// Units are the unit names for the remote application that need to be
	// created as synthetic units. These are extracted from relation endpoints
	// during migration import.
	Units []string
}

// RemoteApplicationOffererImport contains details to import a remote
// application offerer during migration. This represents a remote application
// that this model is consuming from another model.
type RemoteApplicationOffererImport struct {
	RemoteApplicationImport

	// OffererApplicationUUID is the UUID of the application in the offerer
	// model that is offering this application. This is used to link the
	// consumer application to the correct offerer application in the offerer
	// model.
	OffererApplicationUUID application.UUID
}

// RemoteApplicationConsumerImport contains details to import a remote
// application consumer during migration. This represents a remote application
// that this model is offering from another model.
type RemoteApplicationConsumerImport struct {
	RemoteApplicationImport

	// RelationUUID is the UUID of the relation created for this remote
	// application consumer.
	RelationUUID string

	// RelationKey is the key of the relation created for this remote
	// application consumer.
	RelationKey relation.Key

	// ConsumerModelUUID is the UUID of the model consuming the application.
	ConsumerModelUUID string

	// ConsumerApplicationUUID is the synthetic application UUID created in the
	// consumer model to represent this remote application.
	ConsumerApplicationUUID string

	// UserName is the name of the user who made the original offer connection
	// request.
	UserName string
}

// GrantedSecretConsumerImport contains details to import a granted secret
// consumer during migration. These are used to track down which unit has access
// to which revision of a granted secret.
type GrantedSecretConsumerImport struct {
	// Unit is the unit name of the consuming unit.
	Unit unit.Name

	// Revision is the revision of the secret that the unit is consuming.
	CurrentRevision int
}

// GrantedSecretACLImport contains details to import a granted secret ACL during
// migration.
type GrantedSecretACLImport struct {
	// ApplicationName is the name of the application to which the secret is
	// granted.
	ApplicationName string

	// RelationKey represents the unique identifier of a relation
	// through which the secret is granted.
	RelationKey relation.Key

	// Role defines the access role for a secret within the permissions of
	// a granted secret ACL.
	Role secrets.SecretRole
}

// GrantedSecretImport contains details to import a granted secret during
// migration. These secrets are secrets granted by offerer applications to
// consumer applications in the offerer model.
type GrantedSecretImport struct {
	// SecretID is the ID of the secret being granted.
	SecretID string

	// ACLs is a list of applications that have access to the
	// secret through a relation.
	ACLs []GrantedSecretACLImport

	// Consumers is a list of units that actually consumes the secret.
	Consumers []GrantedSecretConsumerImport
}

// RemoteSecretImport contains details to import a remote secret during
// migration. These secrets are secrets granted by offerer applications to
// consumer applications in the consumer model.
type RemoteSecretImport struct {

	// SecretID is the ID of the remote secret
	SecretID string

	// SourceUUID is the UUID of the application offering the secret
	SourceUUID string

	// Label is the label of the remote secret
	Label string

	// ConsumerUnit is the unit name of the consumer unit
	ConsumerUnit unit.Name

	// CurrentRevision is the consumed revision of the remote secret
	CurrentRevision int

	// LatestRevision is the latest revision of the remote secret
	LatestRevision int
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
			return internalerrors.Errorf("constructing remote application offerer for %q: %w", rApp.Name, err)
		}
		offerers = append(offerers, offerer)
	}

	if err := s.modelState.ImportRemoteApplicationOfferers(ctx, offerers); err != nil {
		return internalerrors.Capture(err)
	}

	return nil
}

func (s *MigrationService) constructApplicationOfferer(rApp RemoteApplicationOffererImport) (crossmodelrelation.RemoteApplicationOffererImport, error) {
	synthCharm, err := s.constructOffererSyntheticCharm(rApp.Name, rApp.Endpoints)
	if err != nil {
		return crossmodelrelation.RemoteApplicationOffererImport{}, internalerrors.Errorf(
			"constructing synthetic charm: %w", err)
	}

	return crossmodelrelation.RemoteApplicationOffererImport{
		RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
			Name:                   rApp.Name,
			OfferUUID:              rApp.OfferUUID,
			URL:                    rApp.URL,
			SourceModelUUID:        rApp.SourceModelUUID,
			Macaroon:               rApp.Macaroon,
			Units:                  rApp.Units,
			SyntheticCharm:         synthCharm,
			OffererApplicationUUID: rApp.OffererApplicationUUID.String(),
		},
	}, nil
}

// ImportRemoteApplicationConsumers adds remote application consumers being
// migrated to the current model. These are applications that this model is
// offering to other models.
func (s *MigrationService) ImportRemoteApplicationConsumers(ctx context.Context, imports []RemoteApplicationConsumerImport) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	consumers := make([]crossmodelrelation.RemoteApplicationConsumerImport, 0, len(imports))
	for _, rApp := range imports {
		consumer, err := s.constructApplicationConsumer(ctx, rApp)
		if err != nil {
			return internalerrors.Errorf("constructing remote application consumer for %q: %w", rApp.Name, err)
		}
		consumers = append(consumers, consumer)
	}

	if err := s.modelState.ImportRemoteApplicationConsumers(ctx, consumers); err != nil {
		return internalerrors.Capture(err)
	}

	return nil
}

func (s *MigrationService) constructApplicationConsumer(ctx context.Context, rApp RemoteApplicationConsumerImport) (crossmodelrelation.RemoteApplicationConsumerImport, error) {
	if err := rApp.RelationKey.Validate(); err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf(
			"validating relation key: %w", err).Add(errors.NotValid)
	}

	if err := relation.UUID(rApp.RelationUUID).Validate(); err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf(
			"validating relation UUID: %w", err).Add(errors.NotValid)
	}
	if err := offer.UUID(rApp.OfferUUID).Validate(); err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf(
			"validating offer UUID: %w", err).Add(errors.NotValid)
	}
	if err := model.UUID(rApp.ConsumerModelUUID).Validate(); err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf(
			"validating consumer model UUID: %w", err).Add(errors.NotValid)
	}
	if err := application.UUID(rApp.ConsumerApplicationUUID).Validate(); err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf(
			"validating consumer application UUID: %w", err).Add(errors.NotValid)
	}

	synthCharm, err := s.constructConsumedSyntheticCharm(rApp.Name, rApp.Endpoints)
	if err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf(
			"constructing synthetic charm: %w", err)
	}

	// We require exactly two entries in the relation key: one for the offering
	// application endpoint, and one for the consuming application endpoint.
	// Peer relations are not supported for cross model relations.
	if len(rApp.RelationKey) != 2 {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf(
			"invalid relation key length %d", len(rApp.RelationKey))
	}

	// We can now extract the offering and consuming application endpoints
	// from the relation key.
	var (
		offererApplicationEndpoint  string
		consumerApplicationEndpoint string

		offererApplicationName string
	)
	for _, ep := range rApp.RelationKey {
		if ep.ApplicationName == rApp.Name {
			consumerApplicationEndpoint = ep.EndpointName
		} else {
			offererApplicationName = ep.ApplicationName
			offererApplicationEndpoint = ep.EndpointName
		}
	}

	// The offerer application is only known by its name in the relation key.
	// We need to look up its UUID in the offerer model, which is the current
	// model.
	offererApplicationUUID, err := s.modelState.GetApplicationUUIDByName(ctx, offererApplicationName)
	if err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf(
			"getting offerer application UUID by name %q: %w", offererApplicationName, err)
	}

	charmUUID, err := uuid.NewUUID()
	if err != nil {
		return crossmodelrelation.RemoteApplicationConsumerImport{}, internalerrors.Errorf("generating charm UUID: %w", err)
	}

	return crossmodelrelation.RemoteApplicationConsumerImport{
		RemoteApplicationImport: crossmodelrelation.RemoteApplicationImport{
			Name:                   rApp.Name,
			OfferUUID:              rApp.OfferUUID,
			URL:                    rApp.URL,
			Macaroon:               rApp.Macaroon,
			Units:                  rApp.Units,
			SyntheticCharm:         synthCharm,
			OffererApplicationUUID: offererApplicationUUID,
		},
		RelationUUID:                rApp.RelationUUID,
		ConsumerModelUUID:           rApp.ConsumerModelUUID,
		ConsumerApplicationUUID:     rApp.ConsumerApplicationUUID,
		ConsumerApplicationEndpoint: consumerApplicationEndpoint,
		OffererApplicationEndpoint:  offererApplicationEndpoint,
		UserName:                    rApp.UserName,
		SyntheticCharmUUID:          charmUUID.String(),
	}, nil
}

func (s *MigrationService) constructOffererSyntheticCharm(appName string, endpoints []crossmodelrelation.RemoteApplicationEndpoint) (charm.Charm, error) {
	if len(endpoints) == 0 {
		return charm.Charm{}, internalerrors.Errorf("no endpoints provided for synthetic charm")
	}

	syntheticCharm, err := constructSyntheticCharm(appName, transform.Slice(endpoints, func(ep crossmodelrelation.RemoteApplicationEndpoint) charm.Relation {
		return charm.Relation{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
			Scope:     charm.ScopeGlobal,
		}
	}))
	if err != nil {
		return charm.Charm{}, internalerrors.Errorf("constructing synthetic charm for %q: %w", appName, err)
	}

	return syntheticCharm, nil
}

func (s *MigrationService) constructConsumedSyntheticCharm(appName string, endpoints []crossmodelrelation.RemoteApplicationEndpoint) (charm.Charm, error) {
	if len(endpoints) == 0 {
		return charm.Charm{}, internalerrors.Errorf("no endpoints provided for synthetic charm")
	}

	syntheticCharm, err := constructSyntheticCharm(appName, transform.Slice(endpoints, func(ep crossmodelrelation.RemoteApplicationEndpoint) charm.Relation {
		return charm.Relation{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
		}
	}))
	if err != nil {
		return charm.Charm{}, internalerrors.Errorf("constructing synthetic charm for %q: %w", appName, err)
	}

	// Check that the charm has only one endpoint. There can be multiple
	// synthetic applications per offer, but only one endpoint per synthetic
	// application. To do otherwise requires design and facade changes.
	if err := synthCharmHasOnlyOneEndpoint(endpoints[0].Name, syntheticCharm); err != nil {
		return charm.Charm{}, internalerrors.Errorf("adding consumed relation: %w", err)
	}

	return syntheticCharm, nil
}

// ImportGrantedSecrets imports secrets granted by offerer applications to
// consumer applications in the offerer model.
func (s *MigrationService) ImportGrantedSecrets(ctx context.Context, grantedSecrets []GrantedSecretImport) error {
	return nil
}

// ImportRemoteSecrets imports secrets granted by offerer applications to
// consumer applications in the consumer model.
func (s *MigrationService) ImportRemoteSecrets(ctx context.Context, remoteSecrets []RemoteSecretImport) error {
	return nil
}
