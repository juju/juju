// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"gopkg.in/macaroon.v2"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/status"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ModelRemoteApplicationState describes retrieval and persistence methods for
// cross model relations in the model database.
type ModelRemoteApplicationState interface {
	// AddRemoteApplicationOfferer adds a new synthetic application representing
	// an offer from an external model, to this, the consuming model.
	AddRemoteApplicationOfferer(
		context.Context,
		string,
		crossmodelrelation.AddRemoteApplicationOffererArgs,
	) error

	// AddRemoteApplicationConsumer adds a new synthetic application representing
	// the remote relation on the consuming model, to this, the offering model.
	AddRemoteApplicationConsumer(
		context.Context,
		string,
		crossmodelrelation.AddRemoteApplicationConsumerArgs,
	) error

	// GetRemoteApplicationOfferers returns all the current non-dead remote
	// application offerers in the local model.
	GetRemoteApplicationOfferers(context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error)

	// GetRemoteApplicationConsumers returns all the current non-dead remote
	// application consumers in the local model.
	GetRemoteApplicationConsumers(context.Context) ([]crossmodelrelation.RemoteApplicationConsumer, error)

	// NamespaceRemoteApplicationOfferers returns the database namespace
	// for remote application offerers.
	NamespaceRemoteApplicationOfferers() string

	// NamespaceRemoteApplicationConsumers returns the database namespace
	// for remote application consumers.
	NamespaceRemoteApplicationConsumers() string

	// NamespaceRemoteConsumerRelations returns the remote consumer relations
	// namespace (i.e. the relations table).
	NamespaceRemoteConsumerRelations() string

	// SaveMacaroonForRelation saves the given macaroon for the specified
	// remote application.
	SaveMacaroonForRelation(context.Context, string, []byte) error

	// GetApplicationNameAndUUIDByOfferUUID returns the application name and UUID
	// for the given offer UUID.
	// Returns [applicationerrors.ApplicationNotFound] if the offer or associated
	// application is not found.
	GetApplicationNameAndUUIDByOfferUUID(ctx context.Context, offerUUID string) (string, coreapplication.UUID, error)

	// EnsureUnitsExist ensures that the given synthetic units exist in the local
	// model.
	EnsureUnitsExist(ctx context.Context, appUUID string, units []string) error
}

// AddRemoteApplicationOfferer adds a new synthetic application representing
// an offer from an external model, to this, the consuming model.
func (s *Service) AddRemoteApplicationOfferer(ctx context.Context, applicationName string, args AddRemoteApplicationOffererArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !application.IsValidApplicationName(applicationName) {
		return applicationerrors.ApplicationNameNotValid
	}
	if err := args.OfferUUID.Validate(); err != nil {
		return internalerrors.Errorf("validating offer UUID: %w", err)
	}
	if !uuid.IsValidUUIDString(args.OffererModelUUID) {
		return internalerrors.Errorf("offerer model UUID %q is not a valid UUID", args.OffererModelUUID).Add(errors.NotValid)
	}
	if args.Macaroon == nil {
		return internalerrors.New("macaroon cannot be nil").Add(errors.NotValid)
	}

	// Construct a synthetic charm to represent the remote application charm,
	// so we can track the endpoints it offers.
	syntheticCharm, err := constructSyntheticCharm(applicationName, args.Endpoints)
	if err != nil {
		return internalerrors.Capture(err)
	}

	encodedMacaroon, err := args.Macaroon.MarshalJSON()
	if err != nil {
		return internalerrors.Errorf("marshalling macaroon: %w", err)
	}

	remoteApplicationUUID, err := coreremoteapplication.NewUUID()
	if err != nil {
		return internalerrors.Errorf("creating remote application uuid: %w", err)
	}

	applicationUUID, err := coreapplication.NewID()
	if err != nil {
		return internalerrors.Errorf("creating application uuid: %w", err)
	}

	charmUUID, err := corecharm.NewID()
	if err != nil {
		return internalerrors.Errorf("creating charm uuid: %w", err)
	}

	if err := s.modelState.AddRemoteApplicationOfferer(ctx, applicationName, crossmodelrelation.AddRemoteApplicationOffererArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			RemoteApplicationUUID: remoteApplicationUUID.String(),
			ApplicationUUID:       applicationUUID.String(),
			CharmUUID:             charmUUID.String(),
			Charm:                 syntheticCharm,
			OfferUUID:             args.OfferUUID.String(),
		},
		OfferURL:              args.OfferURL.String(),
		OffererControllerUUID: args.OffererControllerUUID,
		OffererModelUUID:      args.OffererModelUUID,
		EncodedMacaroon:       encodedMacaroon,
	}); err != nil {
		return internalerrors.Errorf("inserting remote application offerer: %w", err)
	}

	s.recordInitRemoteApplicationStatusHistory(ctx, applicationName)

	return nil
}

// AddRemoteApplicationConsumer adds a new synthetic application representing
// a remote relation on the consuming model, to this, the offering model.
func (s *Service) AddRemoteApplicationConsumer(ctx context.Context, args AddRemoteApplicationConsumerArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !uuid.IsValidUUIDString(args.RemoteApplicationUUID) {
		return internalerrors.Errorf("remote application UUID %q is not a valid UUID", args.RemoteApplicationUUID).Add(errors.NotValid)
	}

	// The synthetic application name is prefixed with "remote-" to avoid
	// name clashes with local applications.
	synthApplicationName := "remote-" + strings.Replace(args.RemoteApplicationUUID, "-", "", -1)
	if !application.IsValidApplicationName(synthApplicationName) {
		return applicationerrors.ApplicationNameNotValid
	}
	if err := args.OfferUUID.Validate(); err != nil {
		return internalerrors.Errorf("validating offer UUID: %w", err)
	}
	if !uuid.IsValidUUIDString(args.RelationUUID) {
		return internalerrors.Errorf("relation UUID %q is not a valid UUID", args.RelationUUID).Add(errors.NotValid)
	}
	if !uuid.IsValidUUIDString(args.ConsumerModelUUID) {
		return internalerrors.Errorf("consumer model UUID %q is not a valid UUID", args.ConsumerModelUUID).Add(errors.NotValid)
	}

	// Construct a synthetic charm to represent the remote application charm,
	// so we can track the endpoints it offers.
	syntheticCharm, err := constructSyntheticCharm(synthApplicationName, args.Endpoints)
	if err != nil {
		return internalerrors.Capture(err)
	}

	remoteApplicationUUID, err := coreremoteapplication.NewUUID()
	if err != nil {
		return internalerrors.Errorf("creating remote application uuid: %w", err)
	}

	charmUUID, err := corecharm.NewID()
	if err != nil {
		return internalerrors.Errorf("creating charm uuid: %w", err)
	}

	if err := s.modelState.AddRemoteApplicationConsumer(ctx, synthApplicationName, crossmodelrelation.AddRemoteApplicationConsumerArgs{
		AddRemoteApplicationArgs: crossmodelrelation.AddRemoteApplicationArgs{
			RemoteApplicationUUID: remoteApplicationUUID.String(),
			// NOTE: We use the same UUID as in the remote (consuming) model for
			// the synthetic application we are creating in the offering model.
			// We can do that because we know it's a valid UUID at this point.
			ApplicationUUID:   remoteApplicationUUID.String(),
			CharmUUID:         charmUUID.String(),
			Charm:             syntheticCharm,
			OfferUUID:         args.OfferUUID.String(),
			ConsumerModelUUID: args.ConsumerModelUUID,
		},
		RelationUUID: args.RelationUUID,
	}); internalerrors.Is(err, crossmodelrelationerrors.RemoteRelationAlreadyRegistered) {
		// This can happen if the remote relation was already registered.
		// The method is idempotent, so we just return nil with a debug log.
		s.logger.Debugf(ctx, "remote relation with consumer relation UUID %q already registered", args.RelationUUID)
		return nil
	} else if err != nil {
		return internalerrors.Errorf("inserting remote application consumer: %w", err)
	}

	s.recordInitRemoteApplicationStatusHistory(ctx, synthApplicationName)

	return nil
}

// recordInitRemoteApplicationStatusHistory records the initial status history
// for the remote application. The status is set to Unknown, and the Since time
// is set to the current time.
func (s *Service) recordInitRemoteApplicationStatusHistory(
	ctx context.Context,
	applicationName string,
) {
	statusInfo := corestatus.StatusInfo{
		Status: corestatus.Unknown,
		Since:  ptr(s.clock.Now()),
	}

	if err := s.statusHistory.RecordStatus(ctx, status.RemoteApplication.WithID(applicationName), statusInfo); err != nil {
		s.logger.Warningf(ctx, "recording remote application %q status history: %w", applicationName, err)
	}
}

// GetRemoteApplicationOfferers returns all the current non-dead remote
// application offerers in the local model.
func (s *Service) GetRemoteApplicationOfferers(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.GetRemoteApplicationOfferers(ctx)
}

// GetRemoteApplicationConsumers returns the current state of all remote
// application consumers in the local model.
func (s *Service) GetRemoteApplicationConsumers(ctx context.Context) ([]crossmodelrelation.RemoteApplicationConsumer, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.GetRemoteApplicationConsumers(ctx)
}

// ConsumeRemoteSecretChanges applies secret changes received
// from a remote model to the local model.
func (s *Service) ConsumeRemoteSecretChanges(context.Context) error {
	return nil
}

// EnsureUnitsExist ensures that the given synthetic units exist in the local
// model.
func (s *Service) EnsureUnitsExist(ctx context.Context, appUUID coreapplication.UUID, units []unit.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(units) == 0 {
		return nil
	}

	if err := appUUID.Validate(); err != nil {
		return internalerrors.Errorf(
			"%w:%w", relationerrors.ApplicationUUIDNotValid, err)
	}
	for _, u := range units {
		if err := u.Validate(); err != nil {
			return internalerrors.Capture(err)
		}
	}

	unitNames := make([]string, len(units))
	for i, u := range units {
		unitNames[i] = u.String()
	}

	return s.modelState.EnsureUnitsExist(ctx, appUUID.String(), unitNames)
}

// SuspendRelation suspends the specified relation in the local model
// with the given reason. This will also update the status of the associated
// synthetic application to Error with the given reason.
func (s *Service) SuspendRelation(ctx context.Context, appUUID coreapplication.UUID, relUUID corerelation.UUID, reason string) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appUUID.Validate(); err != nil {
		return internalerrors.Errorf(
			"suspending relation: %w", err).Add(relationerrors.ApplicationUUIDNotValid)
	}
	if err := relUUID.Validate(); err != nil {
		return internalerrors.Errorf(
			"suspending relation: %w", err).Add(relationerrors.RelationUUIDNotValid)
	}

	return nil
}

// SuspendRelation suspends the specified relation in the local model
// with the given reason.
func (s *Service) SetRelationSuspendedState(ctx context.Context, appUUID coreapplication.UUID, relUUID corerelation.UUID, suspended bool, reason string) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appUUID.Validate(); err != nil {
		return internalerrors.Errorf(
			"setting relation suspended state: %w", err).Add(relationerrors.ApplicationUUIDNotValid)
	}
	if err := relUUID.Validate(); err != nil {
		return internalerrors.Errorf(
			"setting relation suspended state: %w", err).Add(relationerrors.RelationUUIDNotValid)
	}

	return nil
}

// SaveMacaroonForRelation saves the given macaroon for the specified remote
// application.
func (s *Service) SaveMacaroonForRelation(ctx context.Context, relationUUID corerelation.UUID, mac *macaroon.Macaroon) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return internalerrors.Errorf("relation UUID %q is not valid: %w", relationUUID, err).Add(errors.NotValid)
	}
	if mac == nil {
		return internalerrors.New("macaroon cannot be nil").Add(errors.NotValid)
	}

	bytes, err := mac.MarshalJSON()
	if err != nil {
		return internalerrors.Errorf("marshalling macaroon: %w", err)
	}

	return s.modelState.SaveMacaroonForRelation(ctx, relationUUID.String(), bytes)
}

// GetRelationToken returns the token associated with the provided relation Key.
// Not implemented yet in the domain service.
func (w *Service) GetRelationToken(ctx context.Context, relationKey string) (string, error) {
	return "", internalerrors.Errorf("crossmodelrelation.GetToken").Add(errors.NotImplemented)
}

// GetApplicationNameAndUUIDByOfferUUID returns the application name and UUID
// for the given offer UUID.
// Returns crossmodelrelationerrors.OfferNotFound if the offer or associated
// application is not found.
func (s *Service) GetApplicationNameAndUUIDByOfferUUID(ctx context.Context, offerUUID offer.UUID) (string, coreapplication.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := offerUUID.Validate(); err != nil {
		return "", "", internalerrors.Errorf("validating offer UUID: %w", err)
	}

	appName, appUUID, err := s.modelState.GetApplicationNameAndUUIDByOfferUUID(ctx, offerUUID.String())
	if err != nil {
		return "", "", internalerrors.Capture(err)
	}
	return appName, appUUID, nil
}

func constructSyntheticCharm(applicationName string, endpoints []charm.Relation) (charm.Charm, error) {
	if len(endpoints) == 0 {
		return charm.Charm{}, internalerrors.New("endpoints cannot be empty").Add(errors.NotValid)
	}
	// Ensure that we don't have any endpoints that are non-global scope.
	for _, endpoint := range endpoints {
		if endpoint.Scope == "" {
			continue
		}
		if endpoint.Scope != charm.ScopeGlobal {
			return charm.Charm{}, internalerrors.Errorf("endpoint %q has non-global scope %q", endpoint.Name, endpoint.Scope).Add(errors.NotValid)
		}
	}

	provides, requires, err := splitRelationsByType(endpoints)
	if err != nil {
		return charm.Charm{}, internalerrors.Errorf("parsing relations by type: %w", err)
	}

	return charm.Charm{
		Metadata: charm.Metadata{
			Name:        applicationName,
			Description: "remote offerer application",
			Provides:    provides,
			Requires:    requires,
		},
		ReferenceName: applicationName,
		Source:        charm.CMRSource,
	}, nil
}

func splitRelationsByType(relations []charm.Relation) (map[string]charm.Relation, map[string]charm.Relation, error) {
	var (
		provides = make(map[string]charm.Relation)
		requires = make(map[string]charm.Relation)
	)
	for _, relation := range relations {
		switch relation.Role {
		case charm.RoleProvider:
			provides[relation.Name] = relation
		case charm.RoleRequirer:
			requires[relation.Name] = relation
		case charm.RolePeer:
			// Peer relations are not supported in CMR, as they represent
			// intra-model relations.
			continue
		default:
			return nil, nil, internalerrors.Errorf("unknown relation role type: %q", relation.Role)
		}
	}

	return provides, requires, nil
}
