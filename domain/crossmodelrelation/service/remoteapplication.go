// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
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

	remoteApplicationUUID, err := uuid.NewUUID()
	if err != nil {
		return internalerrors.Errorf("creating remote application uuid: %w", err)
	}

	applicationUUID, err := uuid.NewUUID()
	if err != nil {
		return internalerrors.Errorf("creating application uuid: %w", err)
	}

	charmUUID, err := uuid.NewUUID()
	if err != nil {
		return internalerrors.Errorf("creating charm uuid: %w", err)
	}

	return s.modelState.AddRemoteApplicationOfferer(ctx, applicationName, crossmodelrelation.AddRemoteApplicationOffererArgs{
		RemoteApplicationUUID: remoteApplicationUUID.String(),
		ApplicationUUID:       applicationUUID.String(),
		CharmUUID:             charmUUID.String(),
		Charm:                 syntheticCharm,
		OfferUUID:             args.OfferUUID,
		OffererControllerUUID: args.OffererControllerUUID,
		OffererModelUUID:      args.OffererModelUUID,
		EncodedMacaroon:       encodedMacaroon,
	})
}

// GetRemoteApplicationConsumers returns the current state of all remote
// application consumers in the local model.
func (s *Service) GetRemoteApplicationConsumers(ctx context.Context) ([]crossmodelrelation.RemoteApplicationConsumer, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return nil, errors.NotImplemented
}

// GetRemoteApplicationOfferers returns all application proxies for offers
// consumed in this model.
func (s *Service) GetRemoteApplicationOfferers(ctx context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return nil, errors.NotImplemented
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
