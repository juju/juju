// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"gopkg.in/macaroon.v2"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/errors"
	corerelation "github.com/juju/juju/core/relation"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
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

	// GetRemoteApplicationOfferers returns all the current non-dead remote
	// application offerers in the local model.
	GetRemoteApplicationOfferers(context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error)

	// NamespaceRemoteApplicationOfferers returns the database namespace
	// for remote application offerers.
	NamespaceRemoteApplicationOfferers() string

	// SaveMacaroonForRelation saves the given macaroon for the specified
	// remote application.
	SaveMacaroonForRelation(context.Context, string, []byte) error
}

// AddRemoteApplicationOfferer adds a new synthetic application representing
// an offer from an external model, to this, the consuming model.
func (s *Service) AddRemoteApplicationOfferer(ctx context.Context, applicationName string, args AddRemoteApplicationOffererArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !application.IsValidApplicationName(applicationName) {
		return applicationerrors.ApplicationNameNotValid
	}
	if !uuid.IsValidUUIDString(args.OfferUUID) {
		return internalerrors.Errorf("offer UUID %q is not a valid UUID", args.OfferUUID).Add(errors.NotValid)
	}
	if !uuid.IsValidUUIDString(args.OffererModelUUID) {
		return internalerrors.Errorf("offerer model UUID %q is not a valid UUID", args.OffererModelUUID).Add(errors.NotValid)
	}
	if args.Macaroon == nil {
		return internalerrors.New("macaroon cannot be nil").Add(errors.NotValid)
	}
	if len(args.Endpoints) == 0 {
		return internalerrors.New("endpoints cannot be empty").Add(errors.NotValid)
	}
	// Ensure that we don't have any endpoints that are non-global scope.
	for _, endpoint := range args.Endpoints {
		if endpoint.Scope == "" {
			continue
		}
		if endpoint.Scope != charm.ScopeGlobal {
			return internalerrors.Errorf("endpoint %q has non-global scope %q", endpoint.Name, endpoint.Scope).Add(errors.NotValid)
		}
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
			OfferUUID:             args.OfferUUID,
		},
		OffererControllerUUID: args.OffererControllerUUID,
		OffererModelUUID:      args.OffererModelUUID,
		EncodedMacaroon:       encodedMacaroon,
	}); err != nil {
		return internalerrors.Errorf("inserting remote application offerer: %w", err)
	}

	s.recordInitRemoteApplicationStatusHistory(ctx, applicationName)

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
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return nil, errors.NotImplemented
}

// SetRemoteApplicationOffererStatus sets the status of the specified remote
// application in the local model.
func (s *Service) SetRemoteApplicationOffererStatus(context.Context, coreapplication.UUID, corestatus.StatusInfo) error {
	return nil
}

// ConsumeRemoteRelationChange applies a relation change event received
// from a remote model to the local model.
func (s *Service) ConsumeRemoteRelationChange(context.Context) error {
	return nil
}

// ConsumeRemoteSecretChanges applies secret changes received
// from a remote model to the local model.
func (s *Service) ConsumeRemoteSecretChanges(context.Context) error {
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

func constructSyntheticCharm(applicationName string, endpoints []charm.Relation) (charm.Charm, error) {
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
