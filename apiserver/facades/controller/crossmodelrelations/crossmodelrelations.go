// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/charm"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	"github.com/juju/juju/internal/macaroon"
	"github.com/juju/juju/rpc/params"
)

// CrossModelRelationsAPIv3 provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPIv3 struct {
	modelUUID       model.UUID
	auth            facade.CrossModelAuthContext
	watcherRegistry facade.WatcherRegistry

	crossModelRelationService CrossModelRelationService
	statusService             StatusService
	secretService             SecretService

	logger logger.Logger
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI(
	modelUUID model.UUID,
	auth facade.CrossModelAuthContext,
	watcherRegistry facade.WatcherRegistry,
	crossModelRelationService CrossModelRelationService,
	statusService StatusService,
	secretService SecretService,
	logger logger.Logger,
) (*CrossModelRelationsAPIv3, error) {
	return &CrossModelRelationsAPIv3{
		modelUUID:                 modelUUID,
		auth:                      auth,
		watcherRegistry:           watcherRegistry,
		crossModelRelationService: crossModelRelationService,
		statusService:             statusService,
		secretService:             secretService,
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
	// Retrieve the application UUID for the provided offer UUID (also validates
	// offer exists).
	offerUUID, err := offer.ParseUUID(relation.OfferUUID)
	if err != nil {
		return nil, err
	}
	sourceModelTag, err := names.ParseModelTag(relation.SourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	appName, appUUID, err := api.crossModelRelationService.GetApplicationNameAndUUIDByOfferUUID(ctx, offerUUID)
	if err != nil {
		return nil, err
	}

	// Check that the supplied macaroon allows access.
	attr, err := api.auth.Authenticator().CheckOfferMacaroons(ctx, api.modelUUID.String(), offerUUID.String(), relation.Macaroons, relation.BakeryVersion)
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
			OfferUUID:             offerUUID,
			RelationUUID:          relation.RelationToken,
			ConsumerModelUUID:     sourceModelTag.Id(),
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

	// Create the relation tag for the remote relation.
	// The relation tag is based on the relation key, which is of the form
	// "app1:epName1 app2:epName2".
	localEndpoint := appName + ":" + relation.LocalEndpointName

	relationKey, err := corerelation.NewKeyFromString(
		strings.Join([]string{localEndpoint, relation.RemoteEndpoint.Name}, " "),
	)
	if err != nil {
		return nil, errors.Annotate(err, "parsing relation key")
	}

	offererRemoteRelationTag := names.NewRelationTag(relationKey.String())

	// Mint a new macaroon attenuated to the actual relation.
	relationMacaroon, err := api.auth.CreateRemoteRelationMacaroon(
		ctx, api.modelUUID, offerUUID.String(), username, offererRemoteRelationTag, relation.BakeryVersion)
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

type OfferWatcher interface {
	watcher.NotifyWatcher
	OfferUUID() offer.UUID
}

type offerWatcherShim struct {
	watcher.NotifyWatcher
	offerUUID offer.UUID
}

func (w *offerWatcherShim) OfferUUID() offer.UUID {
	return w.offerUUID
}

// WatchOfferStatus starts an OfferStatusWatcher for
// watching the status of an offer.
func (api *CrossModelRelationsAPIv3) WatchOfferStatus(
	ctx context.Context,
	offerArgs params.OfferArgs,
) (params.OfferStatusWatchResults, error) {
	results := params.OfferStatusWatchResults{
		Results: make([]params.OfferStatusWatchResult, len(offerArgs.Args)),
	}

	for i, offerArg := range offerArgs.Args {
		offerUUID, err := offer.ParseUUID(offerArg.OfferUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Ensure the supplied macaroon allows access
		_, err = api.auth.Authenticator().CheckOfferMacaroons(ctx, api.modelUUID.String(), offerUUID.String(), offerArg.Macaroons, offerArg.BakeryVersion)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := api.statusService.WatchOfferStatus(ctx, offerUUID)
		if errors.Is(err, crossmodelrelationerrors.OfferNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "offer %q not found", offerArg.OfferUUID)
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watcherID, _, err := internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, &offerWatcherShim{
			NotifyWatcher: w,
			offerUUID:     offerUUID,
		})
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		offerStatus, err := api.statusService.GetOfferStatus(ctx, offerUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i] = params.OfferStatusWatchResult{
			OfferStatusWatcherId: watcherID,
			Changes: []params.OfferStatusChange{
				{
					OfferUUID: offerUUID.String(),
					Status: params.EntityStatus{
						Status: offerStatus.Status,
						Info:   offerStatus.Message,
						Data:   offerStatus.Data,
						Since:  offerStatus.Since,
					},
				},
			},
		}
	}

	return results, nil
}

// WatchConsumedSecretsChanges returns a watcher which notifies of changes to any secrets
// for the specified remote consumers.
func (api *CrossModelRelationsAPIv3) WatchConsumedSecretsChanges(ctx context.Context, args params.WatchRemoteSecretChangesArgs) (params.SecretRevisionWatchResults, error) {
	results := params.SecretRevisionWatchResults{
		Results: make([]params.SecretRevisionWatchResult, len(args.Args)),
	}

	for i, arg := range args.Args {
		var offerUUIDStr string
		// Old clients don't pass in the relation token.
		if arg.RelationToken == "" {
			declared := checkers.InferDeclared(macaroon.MacaroonNamespace, arg.Macaroons)
			offerUUIDStr = declared["offer-uuid"]
		} else {
			offerUUID, err := api.crossModelRelationService.GetOfferUUIDByRelationUUID(ctx, corerelation.UUID(arg.RelationToken))
			if err != nil {
				if errors.Is(err, crossmodelrelationerrors.OfferNotFound) {
					err = apiservererrors.ErrPerm
				}
				results.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			offerUUIDStr = offerUUID.String()
		}

		// Ensure the supplied macaroon allows access.
		_, err := api.auth.Authenticator().CheckOfferMacaroons(ctx, api.modelUUID.String(), offerUUIDStr, arg.Macaroons, arg.BakeryVersion)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := api.crossModelRelationService.WatchRemoteConsumedSecretsChanges(ctx, coreapplication.UUID(arg.ApplicationToken))
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watcherID, uris, err := internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, w)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		changes, err := api.getSecretChanges(ctx, uris)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			_ = worker.Stop(w)
			continue
		}
		results.Results[i] = params.SecretRevisionWatchResult{
			WatcherId: watcherID,
			Changes:   changes,
		}
	}
	return results, nil
}

func (api *CrossModelRelationsAPIv3) getSecretChanges(ctx context.Context, uriStr []string) ([]params.SecretRevisionChange, error) {
	uris := make([]*secrets.URI, len(uriStr))
	for i, s := range uriStr {
		uri, err := secrets.ParseURI(s)
		if err != nil {
			return nil, errors.Trace(err)
		}
		uris[i] = uri
	}
	latest, err := api.secretService.GetLatestRevisions(ctx, uris)
	if err != nil {
		return nil, errors.Trace(err)
	}
	changes := make([]params.SecretRevisionChange, len(uris))
	for i, uri := range uris {
		changes[i] = params.SecretRevisionChange{
			URI:            uri.String(),
			LatestRevision: latest[uri.ID],
		}
	}
	return changes, nil
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
