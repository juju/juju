// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

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
	internalerrors "github.com/juju/juju/internal/errors"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
	"github.com/juju/juju/rpc/params"
)

// CrossModelRelationsAPIv3 provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPIv3 struct {
	modelUUID       model.UUID
	auth            facade.CrossModelAuthContext
	watcherRegistry facade.WatcherRegistry

	applicationService        ApplicationService
	crossModelRelationService CrossModelRelationService
	modelConfigService        ModelConfigService
	relationService           RelationService
	removalService            RemovalService
	secretService             SecretService
	statusService             StatusService

	logger logger.Logger
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI(
	modelUUID model.UUID,
	auth facade.CrossModelAuthContext,
	watcherRegistry facade.WatcherRegistry,
	applicationService ApplicationService,
	crossModelRelationService CrossModelRelationService,
	modelConfigService ModelConfigService,
	relationService RelationService,
	removalService RemovalService,
	secretService SecretService,
	statusService StatusService,
	logger logger.Logger,
) (*CrossModelRelationsAPIv3, error) {
	return &CrossModelRelationsAPIv3{
		modelUUID:                 modelUUID,
		auth:                      auth,
		watcherRegistry:           watcherRegistry,
		applicationService:        applicationService,
		crossModelRelationService: crossModelRelationService,
		modelConfigService:        modelConfigService,
		relationService:           relationService,
		removalService:            removalService,
		secretService:             secretService,
		statusService:             statusService,
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

	relationTag, err := constructRelationTag(relationDetails.Key)
	if err != nil {
		return errors.Annotatef(err, "constructing relation tag for relation %q", relationUUID)
	}

	if err := api.checkMacaroonsForRelation(ctx, relationUUID, relationTag, change.Macaroons, change.BakeryVersion); err != nil {
		return errors.Annotatef(err, "checking macaroons for relation %q", relationUUID)
	}

	applicationUUID, err := api.getApplicationUUIDFromToken(ctx, change.ApplicationOrOfferToken)
	if err != nil {
		return errors.Annotatef(err, "getting application UUID for relation %q", relationUUID)
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
		// Relations only transition to dying and are removed, so we can safely
		// just remove the relation and return early.
		forceCleanup := change.ForceCleanup != nil && *change.ForceCleanup
		_, err := api.removalService.RemoveRemoteRelation(ctx, relationUUID, forceCleanup, 0)
		if errors.Is(err, relationerrors.RelationNotFound) {
			return nil
		}
		return err

	case change.Suspended != nil && *change.Suspended != relationDetails.Suspended:
		if err := api.handleSuspendedRelationChange(ctx, relationUUID, *change.Suspended, change.SuspendedReason); err != nil {
			return err
		}
	}

	if err := api.handlePublishSettings(ctx, relationUUID, applicationUUID, appDetails.Name, change); err != nil {
		return err
	}

	return nil
}

func (api *CrossModelRelationsAPIv3) getApplicationUUIDFromToken(
	ctx context.Context,
	token string,
) (coreapplication.UUID, error) {
	// First try as an application UUID.
	appUUID, err := coreapplication.ParseUUID(token)
	if err == nil {
		return appUUID, nil
	}

	// Next try as an offer UUID.
	offerUUID, err := offer.ParseUUID(token)
	if err != nil {
		return "", errors.NotValidf("token %q is not a valid application or offer UUID", token)
	}

	_, appUUID, err = api.crossModelRelationService.GetApplicationNameAndUUIDByOfferUUID(ctx, offerUUID)
	if err != nil {
		return "", errors.Annotatef(err, "getting application UUID from offer %q", offerUUID)
	}
	return appUUID, nil
}

func (api *CrossModelRelationsAPIv3) handleSuspendedRelationChange(
	ctx context.Context,
	relationUUID corerelation.UUID,
	suspended bool,
	suspendedReason string,
) error {
	// This will check that the relation can be suspended/unsuspended, it
	// must be a cross-model relation.
	if err := api.relationService.SetRemoteRelationSuspendedState(ctx, relationUUID, suspended, suspendedReason); err != nil {
		return errors.Trace(err)
	}

	var relationStatus status.Status
	var message string
	if suspended {
		relationStatus = status.Suspended
		message = suspendedReason
	} else {
		relationStatus = status.Joining
		message = ""
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

func (api *CrossModelRelationsAPIv3) handlePublishSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationUUID coreapplication.UUID,
	applicationName string,
	change params.RemoteRelationChangeEvent,
) error {
	unitSettings, err := api.handleUnitSettings(ctx, applicationUUID, applicationName, change.ChangedUnits)
	if err != nil {
		return errors.Annotatef(err, "handling unit settings for relation %q", relationUUID)
	}

	applicationSettings, err := api.handleApplicationSettings(change.ApplicationSettings)
	if err != nil {
		return errors.Annotatef(err, "handling application settings for relation %q", relationUUID)
	}

	// Process the relation application and unit settings changes.
	if err := api.relationService.SetRelationRemoteApplicationAndUnitSettings(
		ctx,
		applicationUUID,
		relationUUID,
		applicationSettings,
		unitSettings,
	); err != nil {
		return errors.Annotatef(err, "setting application and unit settings %q", relationUUID)
	}

	// We've got departed units, these need to leave scope.
	if err := api.handleDepartedUnits(ctx, relationUUID, change.DepartedUnits, applicationName); err != nil {
		return errors.Annotatef(err, "handling departed units for relation %q", relationUUID)
	}

	return nil
}

func (api *CrossModelRelationsAPIv3) handleUnitSettings(
	ctx context.Context,
	applicationUUID coreapplication.UUID,
	applicationName string,
	unitChanges []params.RemoteRelationUnitChange,
) (map[unit.Name]map[string]string, error) {
	if unitChanges == nil {
		return nil, nil
	}

	units, err := transform.SliceOrErr(unitChanges, func(u params.RemoteRelationUnitChange) (unit.Name, error) {
		return unit.NewNameFromParts(applicationName, u.UnitId)
	})
	if err != nil {
		return nil, errors.Annotatef(err, "parsing unit names")
	}

	// Ensure all the units exist in the local model, we'll need these upfront
	// before we can process the application and unit settings.
	if err := api.crossModelRelationService.EnsureUnitsExist(ctx, applicationUUID, units); err != nil {
		return nil, errors.Annotatef(err, "ensuring units exist")
	}

	// Map the unit settings into a map keyed by unit name.
	unitSettings := make(map[unit.Name]map[string]string, len(unitChanges))
	for i, u := range unitChanges {
		unitName := units[i]

		if u.Settings == nil {
			unitSettings[units[i]] = nil
			continue
		}

		settings := make(map[string]string, len(u.Settings))
		for k, v := range u.Settings {
			switch v := v.(type) {
			case string:
				settings[k] = v
			default:
				return nil, errors.NotValidf("setting value for key %q on unit %q", k, unitName)
			}
		}

		unitSettings[unitName] = settings
	}

	return unitSettings, nil
}

func (api *CrossModelRelationsAPIv3) handleApplicationSettings(
	applicationSettings map[string]any,
) (map[string]string, error) {
	if applicationSettings == nil {
		return nil, nil
	}

	settings := make(map[string]string, len(applicationSettings))
	for k, v := range applicationSettings {
		switch v := v.(type) {
		case string:
			settings[k] = v
		default:
			return nil, errors.NotValidf("application setting value for key %q", k)
		}
	}

	return settings, nil
}

func (api *CrossModelRelationsAPIv3) handleDepartedUnits(
	ctx context.Context,
	relationUUID corerelation.UUID,
	departedUnits []int,
	applicationName string,
) error {
	for _, u := range departedUnits {
		unitName, err := unit.NewNameFromParts(applicationName, u)
		if err != nil {
			return errors.Annotatef(err, "parsing departed unit name %q", u)
		}

		// If the relation unit doesn't exist, then it has already been removed,
		// so we can skip it.
		relationUnitUUID, err := api.relationService.GetRelationUnitUUID(ctx, relationUUID, unitName)
		if errors.Is(err, relationerrors.RelationUnitNotFound) {
			continue
		} else if err != nil {
			return errors.Annotatef(err, "querying relation unit UUID for departed unit %q", unitName)
		}

		if err := api.removalService.LeaveScope(ctx, relationUnitUUID); err != nil {
			return errors.Annotatef(err, "removing departed unit %q", unitName)
		}
	}
	return nil
}

// RegisterRemoteRelations sets up the model to participate
// in the specified relations. This operation is idempotent.
func (api *CrossModelRelationsAPIv3) RegisterRemoteRelations(
	ctx context.Context,
	relations params.RegisterConsumingRelationArgs,
) (params.RegisterConsumingRelationResults, error) {
	results := params.RegisterConsumingRelationResults{
		Results: make([]params.RegisterConsumingRelationResult, len(relations.Relations)),
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
	relation params.RegisterConsumingRelationArg,
) (*params.ConsumingRelationDetails, error) {
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

	_, appUUID, err := api.crossModelRelationService.GetApplicationNameAndUUIDByOfferUUID(ctx, offerUUID)
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
	if err := api.crossModelRelationService.AddConsumedRelation(ctx,
		crossmodelrelationservice.AddConsumedRelationArgs{
			OfferUUID:               offerUUID,
			RelationUUID:            relation.RelationToken,
			OfferingEndpointName:    relation.OfferEndpointName,
			ConsumerModelUUID:       sourceModelTag.Id(),
			ConsumerApplicationUUID: relation.ConsumerApplicationToken,
			// We only have the actual consumed endpoint.
			ConsumerApplicationEndpoint: charm.Relation{
				Name:      relation.ConsumerApplicationEndpoint.Name,
				Role:      charm.RelationRole(relation.ConsumerApplicationEndpoint.Role),
				Interface: relation.ConsumerApplicationEndpoint.Interface,
			},
			Username: username,
		},
	); err != nil {
		return nil, errors.Annotate(err, "adding remote application consumer")
	}

	relationKey, err := api.relationService.GetRelationKeyByUUID(ctx, corerelation.UUID(relation.RelationToken))
	if err != nil {
		return nil, errors.Annotate(err, "getting relation key")
	}

	offererRemoteRelationTag := names.NewRelationTag(relationKey.String())

	// Mint a new macaroon attenuated to the actual relation.
	relationMacaroon, err := api.auth.CreateRemoteRelationMacaroon(
		ctx, api.modelUUID, offerUUID.String(), username, offererRemoteRelationTag, relation.BakeryVersion)
	if err != nil {
		return nil, errors.Annotate(err, "creating relation macaroon")
	}
	return &params.ConsumingRelationDetails{
		// The offering model application UUID is used as the token for the
		// remote model.
		Token:    appUUID.String(),
		Macaroon: relationMacaroon.M(),
	}, nil
}

// WatchRelationChanges starts a RemoteRelationChangesWatcher for each
// specified relation, returning the watcher IDs and initial values,
// or an error if the remote relations couldn't be watched.
func (api *CrossModelRelationsAPIv3) WatchRelationChanges(
	ctx context.Context, remoteRelationArgs params.RemoteEntityArgs,
) (params.RemoteRelationWatchResults, error) {
	results := params.RemoteRelationWatchResults{
		Results: make([]params.RemoteRelationWatchResult, len(remoteRelationArgs.Args)),
	}

	for i, arg := range remoteRelationArgs.Args {
		w, changes, err := api.watchOneRelationChanges(ctx, arg)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// Ignore the changes, this method and the watcher return
		// different structures.
		watcherID, _, err := internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, w)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i].RemoteRelationWatcherId = watcherID
		results.Results[i].Changes = changes
	}

	return results, nil
}

func (api *CrossModelRelationsAPIv3) watchOneRelationChanges(
	ctx context.Context,
	arg params.RemoteEntityArg,
) (RelationChangesWatcher, params.RemoteRelationChangeEvent, error) {
	var empty params.RemoteRelationChangeEvent
	// relationToken is the relation UUID.
	relationToken := arg.Token
	relationUUID := corerelation.UUID(relationToken)

	relationKey, err := api.relationService.GetRelationKeyByUUID(ctx, relationUUID)
	if err != nil {
		return nil, empty, internalerrors.Errorf("getting relation key for %q: %w", relationToken, err)
	}
	relationTag := names.NewRelationTag(relationKey.String())

	if err := api.checkMacaroonsForRelation(ctx, relationUUID, relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
		return nil, empty, internalerrors.Capture(err)
	}

	applicationUUID, err := api.crossModelRelationService.GetOfferingApplicationToken(ctx, relationUUID)
	if err != nil {
		return nil, empty, internalerrors.Errorf("getting offering relation tokens: %w", err)
	}

	w, err := api.relationService.WatchRelationUnits(ctx, relationUUID, applicationUUID)
	if err != nil {
		return nil, empty, internalerrors.Errorf("watching relation units: %w", err)
	}

	change, err := api.getRemoteRelationChangeEvent(ctx, relationUUID, applicationUUID)
	if err != nil {
		return nil, empty, internalerrors.Errorf("getting initial change: %w", err)
	}

	wrapped, err := wrappedRelationChangesWatcher(w, applicationUUID, relationUUID, api.relationService)
	if err != nil {
		return nil, empty, internalerrors.Errorf("watching relation changes: %w", err)
	}

	return wrapped, change, nil
}

func (api *CrossModelRelationsAPIv3) getRemoteRelationChangeEvent(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationUUID coreapplication.UUID,
) (params.RemoteRelationChangeEvent, error) {
	change, err := api.relationService.GetFullRelationUnitChange(ctx, relationUUID, applicationUUID)
	if err != nil {
		return params.RemoteRelationChangeEvent{}, err
	}

	var (
		appSettings  map[string]interface{}
		unitSettings []params.RemoteRelationUnitChange
	)
	if change.ApplicationSettings != nil {
		appSettings = transform.Map(change.ApplicationSettings, func(k string, v string) (string, interface{}) { return k, v })
	}
	if change.UnitsSettings != nil {
		unitSettings = transform.Slice(change.UnitsSettings, func(in relation.UnitSettings) params.RemoteRelationUnitChange {
			return params.RemoteRelationUnitChange{
				UnitId:   in.UnitID,
				Settings: transform.Map(in.Settings, func(k string, v string) (string, interface{}) { return k, v }),
			}
		})
	}

	return params.RemoteRelationChangeEvent{
		RelationToken:           relationUUID.String(),
		ApplicationOrOfferToken: applicationUUID.String(),
		Life:                    change.Life,
		Suspended:               ptr(change.Suspended),
		SuspendedReason:         change.SuspendedReason,
		ApplicationSettings:     appSettings,
		ChangedUnits:            unitSettings,
		InScopeUnits:            change.InScopeUnits,
	}, nil
}

// RelationStatusWatcher contains methods necessary to wrap
// a NotifyWatcher to provide a common RelationStatusWatcher.
type RelationStatusWatcher interface {
	watcher.NotifyWatcher
	RelationUUID() corerelation.UUID
}

type relationStatusWatcherShim struct {
	watcher.NotifyWatcher
	relationUUID corerelation.UUID
}

// RelationUUID returns the relation UUID for the RelationStatusWatcher.
func (w *relationStatusWatcherShim) RelationUUID() corerelation.UUID {
	return w.relationUUID
}

// WatchRelationsSuspendedStatus starts a RelationStatusWatcher for
// watching the life and suspended status of a relation.
func (api *CrossModelRelationsAPIv3) WatchRelationsSuspendedStatus(
	ctx context.Context,
	remoteRelationArgs params.RemoteEntityArgs,
) (params.RelationStatusWatchResults, error) {
	results := params.RelationStatusWatchResults{
		Results: make([]params.RelationLifeSuspendedStatusWatchResult, len(remoteRelationArgs.Args)),
	}

	for i, arg := range remoteRelationArgs.Args {
		relationUUID := corerelation.UUID(arg.Token)
		relationKey, err := api.relationService.GetRelationKeyByUUID(ctx, relationUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(
				internalerrors.Errorf("getting relation key for %q: %w", arg.Token, err))
			continue
		}
		relationTag := names.NewRelationTag(relationKey.String())

		if err := api.checkMacaroonsForRelation(ctx, relationUUID, relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		w, err := api.relationService.WatchRelationLifeSuspendedStatus(ctx, relationUUID)

		if errors.Is(err, relationerrors.RelationNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "relation %q not found", relationUUID)
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watcherID, _, err := internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, &relationStatusWatcherShim{
			NotifyWatcher: w,
			relationUUID:  relationUUID,
		})
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		relationChange, err := api.relationService.GetRelationLifeSuspendedStatus(ctx, relationUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i] = params.RelationLifeSuspendedStatusWatchResult{
			RelationStatusWatcherId: watcherID,
			Changes: []params.RelationLifeSuspendedStatusChange{
				{
					Key:             relationChange.Key,
					Life:            relationChange.Life,
					Suspended:       relationChange.Suspended,
					SuspendedReason: relationChange.SuspendedReason,
				},
			},
		}
	}
	return results, nil
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
	// Secret api does the right thing, but no need to call it unnecessarily.
	if len(uriStr) == 0 {
		return nil, nil
	}
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
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(changes.Changes)),
	}
	for i, change := range changes.Changes {
		api.logger.Debugf(ctx, "publishing ingress network change for relation %q: %+v", change.RelationToken, change)

		relationUUID := corerelation.UUID(change.RelationToken)
		relationDetails, err := api.relationService.GetRelationDetails(ctx, relationUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		relationTag := names.NewRelationTag(relationDetails.Key.String())
		if err := api.checkMacaroonsForRelation(ctx, relationUUID, relationTag, change.Macaroons, change.BakeryVersion); err != nil {
			if errors.Is(err, crossmodelrelationerrors.OfferNotFound) {
				err = apiservererrors.ErrPerm
			}
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// We get the allowed ingress networks from model config for validation.
		modelConfig, err := api.modelConfigService.ModelConfig(ctx)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = api.crossModelRelationService.AddRelationNetworkIngress(ctx, relationUUID, modelConfig.SAASIngressAllow(), change.Networks)
		if errors.Is(err, crossmodelrelationerrors.SubnetNotInWhitelist) {
			results.Results[i].Error = &params.Error{
				Code:    params.CodeForbidden,
				Message: err.Error(),
			}
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}

// WatchEgressAddressesForRelations creates a watcher that notifies when addresses, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func (api *CrossModelRelationsAPIv3) WatchEgressAddressesForRelations(ctx context.Context, remoteRelationArgs params.RemoteEntityArgs) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(remoteRelationArgs.Args)),
	}
	for i, arg := range remoteRelationArgs.Args {
		relationUUID := corerelation.UUID(arg.Token)
		relationKey, err := api.relationService.GetRelationKeyByUUID(ctx, relationUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		relationTag := names.NewRelationTag(relationKey.String())

		if err := api.checkMacaroonsForRelation(ctx, relationUUID, relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			if errors.Is(err, crossmodelrelationerrors.OfferNotFound) {
				err = apiservererrors.ErrPerm
			}
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := api.crossModelRelationService.WatchRelationEgressNetworks(ctx, relationUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watcherID, changes, err := internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, w)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i] = params.StringsWatchResult{
			StringsWatcherId: watcherID,
			Changes:          changes,
		}
	}

	return results, nil
}

func (api *CrossModelRelationsAPIv3) checkMacaroonsForRelation(
	ctx context.Context,
	relationUUID corerelation.UUID,
	relationTag names.RelationTag,
	mac macaroon.Slice,
	version bakery.Version,
) error {
	offerUUID, err := api.crossModelRelationService.GetOfferUUIDByRelationUUID(ctx, relationUUID)
	if err != nil {
		return err
	}

	auth := api.auth.Authenticator()
	return auth.CheckRelationMacaroons(ctx, api.modelUUID.String(), offerUUID.String(), relationTag, mac, version)
}

func constructRelationTag(key corerelation.Key) (names.RelationTag, error) {
	relationKey := key.String()
	if !names.IsValidRelation(relationKey) {
		return names.RelationTag{}, errors.NotValidf("relation key %q", key)
	}
	return names.NewRelationTag(relationKey), nil
}

func ptr[T any](v T) *T {
	return &v
}
