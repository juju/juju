// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ModelDBRemoteApplicationState describes retrieval and persistence methods for
// cross model relations in the model database.
type ModelDBRemoteApplicationState interface {
	// AddRemoteApplicationOfferer adds a new remote application offerer that
	// is on the consumer side of a cross-model relation. This inserts a
	// synthetic application and charm into the model to represent the remote
	// application.
	AddRemoteApplicationOfferer(
		context.Context,
		string,
		crossmodelrelation.AddRemoteApplicationOffererArgs,
	) error
}

// AddRemoteApplicationOffererArgs contains the parameters required to add a new
// remote application offerer.
type AddRemoteApplicationOffererArgs struct {
	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID string

	// OffererControllerUUID is the UUID of the controller that the remote
	// application is in.
	OffererControllerUUID *string

	// OffererModelUUID is the UUID of the model that is offering the
	// application.
	OffererModelUUID string

	// Endpoints is the collection of endpoint names offered.
	Endpoints []charm.Relation

	// Macaroon is the macaroon that the remote application uses to
	// authenticate with the offerer model.
	Macaroon *macaroon.Macaroon
}

// AddRemoteApplicationOfferer adds a new remote application offerer that
// is on the consumer side of a cross-model relation. This enables the tracking
// of the remote application in the local model.
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

	return s.modelState.AddRemoteApplicationOfferer(ctx, applicationName, crossmodelrelation.AddRemoteApplicationOffererArgs{
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
	provides, requires, peers, err := splitRelationsByType(endpoints)
	if err != nil {
		return charm.Charm{}, internalerrors.Errorf("parsing relations by type: %w", err)
	}

	return charm.Charm{
		Metadata: charm.Metadata{
			Name:        applicationName,
			Description: "remote offerer application",
			Provides:    provides,
			Requires:    requires,
			Peers:       peers,
		},
		ReferenceName: applicationName,
		Source:        charm.CMRSource,
	}, nil
}

func splitRelationsByType(relations []charm.Relation) (map[string]charm.Relation, map[string]charm.Relation, map[string]charm.Relation, error) {
	var (
		provides = make(map[string]charm.Relation)
		requires = make(map[string]charm.Relation)
		peers    = make(map[string]charm.Relation)
	)
	for _, relation := range relations {
		switch relation.Role {
		case charm.RoleProvider:
			provides[relation.Name] = relation
		case charm.RoleRequirer:
			requires[relation.Name] = relation
		case charm.RolePeer:
			peers[relation.Name] = relation
		default:
			return nil, nil, nil, internalerrors.Errorf("unknown relation role type: %q", relation.Role)
		}
	}

	return provides, requires, peers, nil
}
