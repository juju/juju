// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	"github.com/juju/juju/rpc/params"
)

// CrossModelRelationsAPIv3 provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPIv3 struct {
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI() (*CrossModelRelationsAPIv3, error) {
	return &CrossModelRelationsAPIv3{}, nil
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
	return params.RegisterRemoteRelationResults{}, nil
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
