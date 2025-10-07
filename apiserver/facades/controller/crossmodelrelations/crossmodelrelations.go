// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application/charm"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	"github.com/juju/juju/rpc/params"
)

// CrossModelRelationsAPIv3 provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPIv3 struct {
	modelUUID model.UUID
	auth      facade.CrossModelAuthContext

	crossModelRelationService CrossModelRelationService

	logger logger.Logger
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI(
	modelUUID model.UUID,
	auth facade.CrossModelAuthContext,
	crossModelRelationService CrossModelRelationService,
	logger logger.Logger,
) (*CrossModelRelationsAPIv3, error) {
	return &CrossModelRelationsAPIv3{
		modelUUID:                 modelUUID,
		auth:                      auth,
		crossModelRelationService: crossModelRelationService,
		logger:                    logger,
	}, nil
}

// PublishRelationChanges publishes relation changes to the
// model hosting the remote application involved in the relation.
func (api *CrossModelRelationsAPIv3) PublishRelationChanges(
	ctx context.Context,
	changes params.RemoteRelationsChanges,
) (params.ErrorResults, error) {
	return params.ErrorResults{}, nil
}

// RegisterRemoteRelations sets up the model to participate
// in the specified relations. This operation is idempotent.
func (api *CrossModelRelationsAPIv3) RegisterRemoteRelations(
	ctx context.Context,
	relations params.RegisterRemoteRelationArgs,
) (params.RegisterRemoteRelationResults, error) {
	results := params.RegisterRemoteRelationResults{
		Results: make([]params.RegisterRemoteRelationResult, len(relations.Relations)),
	}
	for i, relation := range relations.Relations {
		id, err := api.registerOneRemoteRelation(ctx, relation)
		results.Results[i].Result = id
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (api *CrossModelRelationsAPIv3) registerOneRemoteRelation(
	ctx context.Context,
	relation params.RegisterRemoteRelationArg,
) (*params.RemoteRelationDetails, error) {
	// Retrieve the application UUID for the provided offer UUID (also validates offer exists).
	appUUID, err := api.crossModelRelationService.GetApplicationUUIDByOfferUUID(ctx, relation.OfferUUID)
	if err != nil {
		return nil, err
	}

	// Check that the supplied macaroon allows access.
	attr, err := api.auth.Authenticator().CheckOfferMacaroons(ctx, api.modelUUID.String(), relation.OfferUUID, relation.Macaroons, relation.BakeryVersion)
	if err != nil {
		return nil, err
	}
	// The macaroon needs to be attenuated to a user.
	username, ok := attr["username"]
	if username == "" || !ok {
		return nil, apiservererrors.ErrPerm
	}

	// Insert the remote relation.
	if err := api.crossModelRelationService.AddRemoteApplicationConsumer(
		ctx,
		crossmodelrelationservice.AddRemoteApplicationConsumerArgs{
			RemoteApplicationUUID: relation.ApplicationToken,
			OfferUUID:             relation.OfferUUID,
			RelationUUID:          relation.RelationToken,
			// We only have the actual consumed endpoint.
			Endpoints: []charm.Relation{
				{
					Name:      relation.RemoteEndpoint.Name,
					Role:      charm.RelationRole(relation.RemoteEndpoint.Role),
					Interface: relation.RemoteEndpoint.Interface,
				},
			},
		},
	); err != nil {
		return nil, errors.Annotate(err, "adding remote application consumer")
	}

	// Now get the relation UUID of the "synthetic" remote relation.
	offererRemoteRelationUUID, err := api.crossModelRelationService.GetApplicationRemoteRelationByConsumerRelationUUID(ctx, relation.RelationToken)
	if err != nil {
		return nil, errors.Annotate(err, "getting remote relation UUID")
	}
	offererRemoteRelationTag := names.NewRelationTag(offererRemoteRelationUUID.String())

	// Mint a new macaroon attenuated to the actual relation.
	relationMacaroon, err := api.auth.CreateRemoteRelationMacaroon(
		ctx, api.modelUUID, relation.OfferUUID, username, offererRemoteRelationTag, relation.BakeryVersion)
	if err != nil {
		return nil, errors.Annotate(err, "creating relation macaroon")
	}
	return &params.RemoteRelationDetails{
		// The offering model application UUID is used as the token for the
		// remote model.
		Token:    appUUID.String(),
		Macaroon: relationMacaroon.M(),
	}, nil
}

// WatchRelationChanges starts a RemoteRelationChangesWatcher for each
// specified relation, returning the watcher IDs and initial values,
// or an error if the remote relations couldn't be watched.
func (api *CrossModelRelationsAPIv3) WatchRelationChanges(ctx context.Context, remoteRelationArgs params.RemoteEntityArgs) (
	params.RemoteRelationWatchResults, error,
) {
	return params.RemoteRelationWatchResults{}, nil
}

// WatchRelationsSuspendedStatus starts a RelationStatusWatcher for
// watching the life and suspended status of a relation.
func (api *CrossModelRelationsAPIv3) WatchRelationsSuspendedStatus(
	ctx context.Context,
	remoteRelationArgs params.RemoteEntityArgs,
) (params.RelationStatusWatchResults, error) {
	return params.RelationStatusWatchResults{}, nil
}

// WatchOfferStatus starts an OfferStatusWatcher for
// watching the status of an offer.
func (api *CrossModelRelationsAPIv3) WatchOfferStatus(
	ctx context.Context,
	offerArgs params.OfferArgs,
) (params.OfferStatusWatchResults, error) {
	return params.OfferStatusWatchResults{}, nil
}

// WatchConsumedSecretsChanges returns a watcher which notifies of changes to any secrets
// for the specified remote consumers.
func (api *CrossModelRelationsAPIv3) WatchConsumedSecretsChanges(ctx context.Context, args params.WatchRemoteSecretChangesArgs) (params.SecretRevisionWatchResults, error) {
	return params.SecretRevisionWatchResults{}, nil
}

// PublishIngressNetworkChanges publishes changes to the required
// ingress addresses to the model hosting the offer in the relation.
func (api *CrossModelRelationsAPIv3) PublishIngressNetworkChanges(
	ctx context.Context,
	changes params.IngressNetworksChanges,
) (params.ErrorResults, error) {
	return params.ErrorResults{}, nil
}

// WatchEgressAddressesForRelations creates a watcher that notifies when addresses, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func (api *CrossModelRelationsAPIv3) WatchEgressAddressesForRelations(ctx context.Context, remoteRelationArgs params.RemoteEntityArgs) (params.StringsWatchResults, error) {
	return params.StringsWatchResults{}, nil
}
