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
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
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

	// AddConsumedRelation adds a new synthetic application representing
	// the application on the consuming model, to this, the offering model.
	// The synthetic application is used to create a relation with the
	// provided charm.Relation from the consuming side and the offering
	// application endpoint name in the current model.
	AddConsumedRelation(
		context.Context,
		string,
		crossmodelrelation.AddRemoteApplicationConsumerArgs,
	) error

	// GetRemoteApplicationOfferers returns all the current non-dead remote
	// application offerers in the local model.
	GetRemoteApplicationOfferers(context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error)

	// GetRemoteApplicationOffererByApplicationName returns the UUID of the remote
	// application offerer for the given application name.
	GetRemoteApplicationOffererByApplicationName(context.Context, string) (string, error)

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

	// GetMacaroonForRelation gets the macaroon for the specified remote relation,
	// returning an error satisfying [crossmodelrelationerrors.MacaroonNotFound]
	// if the macaroon is not found.
	GetMacaroonForRelation(context.Context, string) (*macaroon.Macaroon, error)

	// GetApplicationNameAndUUIDByOfferUUID returns the application name and UUID
	// for the given offer UUID.
	// Returns [applicationerrors.ApplicationNotFound] if the offer or associated
	// application is not found.
	GetApplicationNameAndUUIDByOfferUUID(ctx context.Context, offerUUID string) (string, coreapplication.UUID, error)

	// EnsureUnitsExist ensures that the given synthetic units exist in the local
	// model.
	EnsureUnitsExist(ctx context.Context, appUUID string, units []string) error

	// IsRelationWithEndpointIdentifiersSuspended returns the suspended status
	// of a relation with the specified endpoints.
	// The following error types can be expected:
	//   - [relationerrors.RelationNotFound]: when no relation exists for the given
	//     endpoints.
	IsRelationWithEndpointIdentifiersSuspended(
		ctx context.Context,
		endpoint1, endpoint2 corerelation.EndpointIdentifier,
	) (bool, error)

	// InitialWatchStatementForConsumerRelations returns the namespace and the
	// initial query function for watching relation UUIDs that are associated with
	// remote offerer applications present in this model (i.e. consumer side).
	InitialWatchStatementForConsumerRelations() (string, eventsource.NamespaceQuery)

	// GetConsumerRelationUUIDs filters the provided relation UUIDs and returns
	// only those that are associated with remote offerer applications in this model.
	GetConsumerRelationUUIDs(ctx context.Context, relationUUIDs ...string) ([]string, error)

	// GetOfferingApplicationToken returns the offering application token (uuid)
	// for the given offer UUID.
	GetOfferingApplicationToken(ctx context.Context, offerUUID string) (string, error)

	// GetOffererRelationUUIDsForConsumers returns the relation UUIDs associated
	// with the provided remote consumer UUIDs.
	GetOffererRelationUUIDsForConsumers(ctx context.Context, consumerUUIDs ...string) ([]string, error)

	// GetAllOffererRelationUUIDs returns all relation UUIDs that are associated
	// with remote consumers in this model (i.e. offerer side relations).
	GetAllOffererRelationUUIDs(ctx context.Context) ([]string, error)

	// InitialWatchStatementForOffererRelations returns the namespace and the
	// initial query function for watching relation UUIDs that are associated with
	// remote consumer applications present in this model (i.e. offerer side).
	InitialWatchStatementForOffererRelations() (string, eventsource.NamespaceQuery)

	// GetOffererModelUUID returns the offering model UUID for a remote application
	// offerer, based on the given application name.
	// The following error types can be expected:
	//   - [crossmodelrelationerrors.RemoteApplicationNotFound]: when the application
	//     is not a remote offerer application.
	GetOffererModelUUID(ctx context.Context, appName string) (coremodel.UUID, error)

	// IsApplicationSynthetic checks if the given application exists in the model
	// and is a synthetic application, based on the charm source being 'cmr'.
	IsApplicationSynthetic(ctx context.Context, appName string) (bool, error)
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

	applicationUUID, err := coreapplication.NewUUID()
	if err != nil {
		return internalerrors.Errorf("creating application uuid: %w", err)
	}

	charmUUID, err := corecharm.NewID()
	if err != nil {
		return internalerrors.Errorf("creating charm uuid: %w", err)
	}

	if err := s.modelState.AddRemoteApplicationOfferer(ctx, applicationName, crossmodelrelation.AddRemoteApplicationOffererArgs{
		RemoteApplicationUUID: remoteApplicationUUID.String(),
		ApplicationUUID:       applicationUUID.String(),
		CharmUUID:             charmUUID.String(),
		Charm:                 syntheticCharm,
		OfferUUID:             args.OfferUUID.String(),
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

// AddConsumedRelation adds a new synthetic application representing
// the application on the consuming model, to this, the offering model.
// The synthetic application is used to create a relation with the
// provided charm.Relation from the consuming side and the offering
// application endpoint name in the current model.
func (s *Service) AddConsumedRelation(ctx context.Context, args AddConsumedRelationArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !uuid.IsValidUUIDString(args.ConsumerApplicationUUID) {
		return internalerrors.Errorf("remote application UUID %q is not a valid UUID", args.ConsumerApplicationUUID).Add(errors.NotValid)
	}

	synthApplicationUUID, err := coreapplication.NewUUID()
	if err != nil {
		return internalerrors.Errorf("creating application uuid: %w", err)
	}

	// The synthetic application name is prefixed with "remote-" to avoid
	// name clashes with local applications.
	synthApplicationName := "remote-" + strings.ReplaceAll(synthApplicationUUID.String(), "-", "")
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

	if args.ConsumerApplicationEndpoint.Name == "" {
		return internalerrors.Errorf("endpoint cannot be empty").Add(errors.NotValid)
	}
	if args.OfferingEndpointName == "" {
		return internalerrors.Errorf("offer endpoint cannot be empty").Add(errors.NotValid)
	}

	// Construct a synthetic charm to represent the remote application charm,
	// so we can track the endpoints it offers.
	syntheticCharm, err := constructSyntheticCharm(synthApplicationName, []charm.Relation{args.ConsumerApplicationEndpoint})
	if err != nil {
		return internalerrors.Capture(err)
	}

	charmUUID, err := corecharm.NewID()
	if err != nil {
		return internalerrors.Errorf("creating charm uuid: %w", err)
	}

	if err := s.modelState.AddConsumedRelation(ctx, synthApplicationName, crossmodelrelation.AddRemoteApplicationConsumerArgs{
		OfferUUID:         args.OfferUUID.String(),
		OfferEndpointName: args.OfferingEndpointName,
		ConsumerModelUUID: args.ConsumerModelUUID,
		RelationUUID:      args.RelationUUID,

		// ConsumerApplicationUUID is the application UUID in the consuming
		// model.
		ConsumerApplicationUUID:     args.ConsumerApplicationUUID,
		ConsumerApplicationEndpoint: args.ConsumerApplicationEndpoint.Name,

		// SynthApplicationUUID is the application UUID created
		// to represent the synthetic application in the offering model. This
		// is randomly generated and can be looked up via the offer and relation
		// uuid.
		SynthApplicationUUID: synthApplicationUUID.String(),

		// CharmUUID and Charm represent the synthetic charm created to
		// represent the remote application on the offering model.
		CharmUUID: charmUUID.String(),
		Charm:     syntheticCharm,

		Username: args.Username,
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

// GetRemoteApplicationOffererByApplicationName returns the UUID of the remote
// application offerer for the given application name.
func (s *Service) GetRemoteApplicationOffererByApplicationName(ctx context.Context, appName string) (coreremoteapplication.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.modelState.GetRemoteApplicationOffererByApplicationName(ctx, appName)
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	ret, err := coreremoteapplication.ParseUUID(uuid)
	if err != nil {
		return "", internalerrors.Errorf("parsing remote application offerer UUID: %w", err)
	}

	return ret, nil
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
			"ensuring units exist: %w", err).Add(applicationerrors.ApplicationUUIDNotValid)
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

// SaveMacaroonForRelation saves the given macaroon for the specified remote
// relation.
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

// GetMacaroonForRelation gets the macaroon for the specified remote relation,
// returning an error satisfying [crossmodelrelationerrors.MacaroonNotFound]
// if the macaroon is not found.
func (s *Service) GetMacaroonForRelation(ctx context.Context, relationUUID corerelation.UUID) (*macaroon.Macaroon, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return nil, internalerrors.Errorf("relation UUID %q is not valid: %w", relationUUID, err).Add(errors.NotValid)
	}
	return s.modelState.GetMacaroonForRelation(ctx, relationUUID.String())
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

// GetOfferingApplicationToken returns the offering application token (UUID)
// for the given relation token (UUID).
func (s *Service) GetOfferingApplicationToken(
	ctx context.Context, relationUUID corerelation.UUID,
) (coreapplication.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return "", internalerrors.Errorf("validating offer UUID: %w", err)
	}

	appUUID, err := s.modelState.GetOfferingApplicationToken(ctx, relationUUID.String())
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	return coreapplication.ParseUUID(appUUID)
}

// IsCrossModelRelationValidForApplication checks that the cross model relation is valid for the application.
// A relation is valid if it is not suspended and the application is involved in the relation.
func (s *Service) IsCrossModelRelationValidForApplication(ctx context.Context, relationKey corerelation.Key, appName string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationKey.Validate(); err != nil {
		return false, relationerrors.RelationKeyNotValid
	}

	eids := relationKey.EndpointIdentifiers()
	if len(eids) != 2 {
		// Should never happen.
		return false, internalerrors.Errorf("internal error: unexpected number of endpoints %d", len(eids))
	}
	if eids[0].ApplicationName != appName && eids[1].ApplicationName != appName {
		return false, internalerrors.Errorf("relation %q not valid for application %q", relationKey, appName).Add(errors.NotValid)
	}

	isSuspended, err := s.modelState.IsRelationWithEndpointIdentifiersSuspended(ctx, eids[0], eids[1])
	if err != nil {
		return false, internalerrors.Errorf("getting relation suspended status by key: %w", err)
	}
	return !isSuspended, nil
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

// IsApplicationSynthetic checks if the given application exists in the model
// and is a synthetic application.
func (s *Service) IsApplicationSynthetic(ctx context.Context, appName string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.IsApplicationSynthetic(ctx, appName)
}

// GetOffererModelUUID returns the offering model UUID, based on a given
// application.
func (s *Service) GetOffererModelUUID(ctx context.Context, appName string) (coremodel.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelUUID, err := s.modelState.GetOffererModelUUID(ctx, appName)
	if err != nil {
		return coremodel.UUID(""), internalerrors.Capture(err)
	}
	return modelUUID, nil
}
