// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
	"github.com/juju/juju/rpc/params"
)

// CrossModelRelationsAPIv3 provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPIv3 struct {
	modelUUID       model.UUID
	auth            facade.CrossModelAuthContext
	watcherRegistry facade.WatcherRegistry

	applicationService        ApplicationService
	relationService           RelationService
	crossModelRelationService CrossModelRelationService
	statusService             StatusService
	secretService             SecretService
	removalService            RemovalService

	logger logger.Logger
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI(
	modelUUID model.UUID,
	auth facade.CrossModelAuthContext,
	watcherRegistry facade.WatcherRegistry,
	applicationService ApplicationService,
	relationService RelationService,
	crossModelRelationService CrossModelRelationService,
	statusService StatusService,
	secretService SecretService,
	removalService RemovalService,
	logger logger.Logger,
) (*CrossModelRelationsAPIv3, error) {
	return &CrossModelRelationsAPIv3{
		modelUUID:                 modelUUID,
		auth:                      auth,
		watcherRegistry:           watcherRegistry,
		applicationService:        applicationService,
		relationService:           relationService,
		crossModelRelationService: crossModelRelationService,
		statusService:             statusService,
		secretService:             secretService,
		removalService:            removalService,
		logger:                    logger,
	}, nil
}

// PublishRelationChanges publishes relation changes to the
// model hosting the remote application involved in the relation.
func (api *CrossModelRelationsAPIv3) PublishRelationChanges(
	ctx context.Context,
	changes params.RemoteRelationsChanges,
) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(changes.Changes)),
	}

	for i, change := range changes.Changes {
		err := api.publishOneRelationChange(ctx, change)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}

	return results, nil
}

func (api *CrossModelRelationsAPIv3) publishOneRelationChange(ctx context.Context, change params.RemoteRelationChangeEvent) error {
	relationUUID, err := corerelation.ParseUUID(change.RelationToken)
	if err != nil {
		return err
	}

	// Ensure that we have a relation and that it isn't dead.
	// If the relation is not found or dead, we simply skip publishing
	// for that relation. This shouldn't bring down the whole operation.
	relationDetails, err := api.relationService.GetRelationDetails(ctx, relationUUID)
	if errors.Is(err, relationerrors.RelationNotFound) {
		api.logger.Debugf(ctx, "relation %q not found when publishing relation changes", change.RelationToken)
		return nil
	} else if err != nil {
		return err
	} else if relationDetails.Life == life.Dead {
		api.logger.Debugf(ctx, "relation %q is dead when publishing relation changes", change.RelationToken)
		return nil
	}

	relationTag, err := constructRelationTag(relationDetails)
	if err != nil {
		return errors.Annotatef(err, "constructing relation tag for relation %q", relationUUID)
	}

	if err := api.checkMacaroonsForRelation(ctx, relationUUID, relationTag, change.Macaroons, change.BakeryVersion); err != nil {
		return errors.Annotatef(err, "checking macaroons for relation %q", relationUUID)
	}

	// TODO (stickupkid): Work out if we get a offer UUID and get the
	// application UUID from that instead.
	applicationUUID, err := coreapplication.ParseID(change.ApplicationOrOfferToken)
	if err != nil {
		return err
	}

	// Ensure that the application is still alive.
	appDetails, err := api.applicationService.GetApplicationDetails(ctx, applicationUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) || appDetails.Life == domainlife.Dead {
		return errors.NotFoundf("application %q not found or dead when publishing relation changes for relation %q", applicationUUID, relationUUID)
	} else if err != nil {
		return err
	}

	switch {
	case change.Life != life.Alive:
		// We're dying or dead, either way we shouldn't continue onwards.
		if err := api.removalService.RemoveRemoteRelation(ctx, relationUUID); err != nil {
			return errors.Annotatef(err, "removing remote relation %q", relationUUID)
		}
		if change.Life == life.Dead {
			// No further processing for dead relations.
			return nil
		}

	case change.Suspended != nil && *change.Suspended != relationDetails.Suspended:
		if err := api.handleSuspendedRelationChange(ctx, relationUUID, *change.Suspended, change.SuspendedReason); err != nil {
			return err
		}
	}

	if err := api.handleUnitSettings(ctx, relationUUID, applicationUUID, appDetails.Name, change); err != nil {
		return err
	}

	return nil
}

func (api *CrossModelRelationsAPIv3) handleSuspendedRelationChange(
	ctx context.Context,
	relationUUID corerelation.UUID,
	suspended bool,
	suspendedReason string,
) error {
	var relationStatus status.Status
	var message string
	if suspended {
		relationStatus = status.Suspended
		message = suspendedReason
	} else {
		relationStatus = status.Joining
		message = ""

		// TODO (stickupkid): Check that the relation is still viable for
		// consumption i.e. you can consume it again.
	}

	return api.statusService.SetRemoteRelationStatus(
		ctx,
		relationUUID,
		status.StatusInfo{
			Status:  relationStatus,
			Message: message,
		},
	)
}

func (api *CrossModelRelationsAPIv3) handleUnitSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationUUID coreapplication.UUID,
	applicationName string,
	change params.RemoteRelationChangeEvent,
) error {
	units, err := transform.SliceOrErr(change.ChangedUnits, func(u params.RemoteRelationUnitChange) (unit.Name, error) {
		return unit.NewNameFromParts(applicationName, u.UnitId)
	})
	if err != nil {
		return errors.Annotatef(err, "parsing unit names for relation %q", relationUUID)
	}

	// Ensure all the units exist in the local model, we'll need these upfront
	// before we can process the application and unit settings.
	if err := api.crossModelRelationService.EnsureUnitsExist(ctx, applicationUUID, units); err != nil {
		return errors.Annotatef(err, "ensuring units exist for relation %q", relationUUID)
	}

	// Map the unit settings into a map keyed by unit name.
	unitSettings := make(map[unit.Name]map[string]string, len(change.ChangedUnits))
	for _, u := range change.ChangedUnits {
		unitName, err := unit.NewNameFromParts(applicationName, u.UnitId)
		if err != nil {
			return errors.Annotatef(err, "parsing unit name %q for relation %q", u.UnitId, relationUUID)
		}

		settings := make(map[string]string, len(u.Settings))
		for k, v := range u.Settings {
			switch v := v.(type) {
			case string:
				settings[k] = v
			default:
				return errors.NotValidf("setting value for key %q on unit %q for relation %q", k, unitName, relationUUID)
			}
		}

		unitSettings[unitName] = settings
	}

	// Process the relation application and unit settings changes.
	if err := api.relationService.SetRelationRemoteApplicationAndUnitSettings(
		ctx,
		applicationUUID,
		relationUUID,
		change.ApplicationSettings,
		unitSettings,
	); err != nil {
		return errors.Annotatef(err, "setting application and unit settings %q", relationUUID)
	}

	// We've got departed units, these need to leave scope.
	for _, u := range change.DepartedUnits {
		unitName, err := unit.NewNameFromParts(applicationName, u)
		if err != nil {
			return errors.Annotatef(err, "parsing departed unit name %q for relation %q", u, relationUUID)
		}

		// If the relation unit doesn't exist, then it has already been removed,
		// so we can skip it.
		relationUnitUUID, err := api.relationService.GetRelationUnitUUID(ctx, relationUUID, unitName)
		if errors.Is(err, relationerrors.RelationUnitNotFound) {
			continue
		} else if err != nil {
			return errors.Annotatef(err, "querying relation unit UUID for departed unit %q for relation %q", unitName, relationUUID)
		}

		if err := api.removalService.LeaveScope(ctx, relationUnitUUID); err != nil {
			return errors.Annotatef(err, "removing departed unit %q for relation %q", unitName, relationUUID)
		}
	}

	return nil
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
	if err := api.crossModelRelationService.AddRemoteApplicationConsumer(ctx,
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
			declared := checkers.InferDeclared(internalmacaroon.MacaroonNamespace, arg.Macaroons)
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

func (api *CrossModelRelationsAPIv3) checkMacaroonsForRelation(
	ctx context.Context,
	relationUUID corerelation.UUID,
	relationTag names.RelationTag,
	mac macaroon.Slice,
	version bakery.Version,
) error {
	offerUUID, err := api.crossModelRelationService.GetOfferUUIDFromRelationUUID(ctx, relationUUID)
	if err != nil {
		return err
	}

	auth := api.auth.Authenticator()
	return auth.CheckRelationMacaroons(ctx, api.modelUUID.String(), offerUUID.String(), relationTag, mac, version)
}

func constructRelationTag(relationDetails relation.RelationDetails) (names.RelationTag, error) {
	relationKey := relationDetails.Key.String()
	if !names.IsValidRelation(relationKey) {
		return names.RelationTag{}, errors.NotValidf("relation key %q", relationDetails.Key)
	}
	return names.NewRelationTag(relationKey), nil
}
