// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"strings"
	"sync"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/kr/pretty"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/charm"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
	internalrelation "github.com/juju/juju/internal/relation"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

type egressAddressWatcherFunc func(facade.Resources, firewall.State, firewall.ModelConfigService, params.Entities) (params.StringsWatchResults, error)
type relationStatusWatcherFunc func(CrossModelRelationsState, names.RelationTag) (state.StringsWatcher, error)
type offerStatusWatcherFunc func(context.Context, CrossModelRelationsState, string) (OfferWatcher, error)
type consumedSecretsWatcherFunc func(context.Context, SecretService, string) (corewatcher.StringsWatcher, error)

// CrossModelRelationsAPIv3 provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPIv3 struct {
	st                 CrossModelRelationsState
	fw                 firewall.State
	secretService      SecretService
	modelConfigService ModelConfigService
	applicationService ApplicationService
	resources          facade.Resources
	authorizer         facade.Authorizer

	mu              sync.Mutex
	authCtxt        *commoncrossmodel.AuthContext
	relationToOffer map[string]string

	egressAddressWatcher   egressAddressWatcherFunc
	relationStatusWatcher  relationStatusWatcherFunc
	offerStatusWatcher     offerStatusWatcherFunc
	consumedSecretsWatcher consumedSecretsWatcherFunc
	logger                 logger.Logger
	modelID                model.UUID
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI(
	modelID model.UUID,
	st CrossModelRelationsState,
	fw firewall.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	authCtxt *commoncrossmodel.AuthContext,
	secretService SecretService,
	modelConfigService ModelConfigService,
	applicationService ApplicationService,
	egressAddressWatcher egressAddressWatcherFunc,
	relationStatusWatcher relationStatusWatcherFunc,
	offerStatusWatcher offerStatusWatcherFunc,
	consumedSecretsWatcher consumedSecretsWatcherFunc,
	logger logger.Logger,
) (*CrossModelRelationsAPIv3, error) {
	return &CrossModelRelationsAPIv3{
		st:                     st,
		fw:                     fw,
		resources:              resources,
		authorizer:             authorizer,
		authCtxt:               authCtxt,
		secretService:          secretService,
		modelConfigService:     modelConfigService,
		applicationService:     applicationService,
		egressAddressWatcher:   egressAddressWatcher,
		relationStatusWatcher:  relationStatusWatcher,
		offerStatusWatcher:     offerStatusWatcher,
		consumedSecretsWatcher: consumedSecretsWatcher,
		relationToOffer:        make(map[string]string),
		logger:                 logger,
		modelID:                modelID,
	}, nil
}

func (api *CrossModelRelationsAPIv3) checkMacaroonsForRelation(ctx context.Context, relationTag names.Tag, mac macaroon.Slice, version bakery.Version) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	offerUUID, ok := api.relationToOffer[relationTag.Id()]
	if !ok {
		oc, err := api.st.OfferConnectionForRelation(relationTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		offerUUID = oc.OfferUUID()
	}
	auth := api.authCtxt.Authenticator()
	return auth.CheckRelationMacaroons(ctx, api.modelID, offerUUID, relationTag, mac, version)
}

// PublishRelationChanges publishes relation changes to the
// model hosting the remote application involved in the relation.
func (api *CrossModelRelationsAPIv3) PublishRelationChanges(
	ctx context.Context,
	changes params.RemoteRelationsChanges,
) (params.ErrorResults, error) {
	api.logger.Debugf(context.TODO(), "PublishRelationChanges: %+v", changes)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(changes.Changes)),
	}
	for i, change := range changes.Changes {
		relationTag, err := api.st.GetRemoteEntity(change.RelationToken)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				api.logger.Debugf(context.TODO(), "no relation tag %+v in model %s, exit early", change.RelationToken, api.modelID)
				continue
			}
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		api.logger.Debugf(context.TODO(), "relation tag for token %+v is %v", change.RelationToken, relationTag)
		if err := api.checkMacaroonsForRelation(ctx, relationTag, change.Macaroons, change.BakeryVersion); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Look up the application on the remote side of this relation
		// ie from the model which published this change.
		appOrOfferTag, err := api.st.GetRemoteEntity(change.ApplicationOrOfferToken)
		if err != nil && !errors.Is(err, errors.NotFound) {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// The tag is either an application tag (consuming side),
		// or an offer tag (offering side).
		var applicationTag names.Tag
		if err == nil {
			switch k := appOrOfferTag.Kind(); k {
			case names.ApplicationTagKind:
				applicationTag = appOrOfferTag
			case names.ApplicationOfferTagKind:
				// For an offer tag, load the offer and get the offered app from that.
				offer, err := api.st.ApplicationOfferForUUID(appOrOfferTag.Id())
				if err != nil && !errors.IsNotFound(err) {
					results.Results[i].Error = apiservererrors.ServerError(err)
					continue
				}
				if err == nil {
					applicationTag = names.NewApplicationTag(offer.ApplicationName)
				}
			default:
				// Should never happen.
				results.Results[i].Error = apiservererrors.ServerError(errors.NotValidf("offer app tag kind %q", k))
				continue
			}
		}
		if err := commoncrossmodel.PublishRelationChange(ctx, api.authorizer, api.st, api.modelID, relationTag, applicationTag, change); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if change.Life != life.Alive {
			delete(api.relationToOffer, relationTag.Id())
		}
	}
	return results, nil
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
		id, err := api.registerRemoteRelation(ctx, relation)
		results.Results[i].Result = id
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (api *CrossModelRelationsAPIv3) registerRemoteRelation(ctx context.Context, relation params.RegisterRemoteRelationArg) (*params.RemoteRelationDetails, error) {
	api.logger.Debugf(context.TODO(), "register remote relation %+v", relation)
	// TODO(wallyworld) - do this as a transaction so the result is atomic
	// Perform some initial validation - is the local application alive?

	// Look up the offer record so get the local application to which we need to relate.
	appOffer, err := api.st.ApplicationOfferForUUID(relation.OfferUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Check that the supplied macaroon allows access.
	auth := api.authCtxt.Authenticator()
	attr, err := auth.CheckOfferMacaroons(ctx, api.modelID, appOffer.OfferUUID, relation.Macaroons, relation.BakeryVersion)
	if err != nil {
		return nil, err
	}
	// The macaroon needs to be attenuated to a user.
	username, ok := attr["username"]
	if username == "" || !ok {
		return nil, apiservererrors.ErrPerm
	}
	localApplicationName := appOffer.ApplicationName
	localApp, err := api.st.Application(localApplicationName)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get application for offer %q", relation.OfferUUID)
	}
	if localApp.Life() != state.Alive {
		// We don't want to leak the application name so just log it.
		api.logger.Warningf(context.TODO(), "local application for offer %v not found", localApplicationName)
		return nil, errors.NotFoundf("local application for offer %v", relation.OfferUUID)
	}
	eps, err := localApp.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Does the requested local endpoint exist?
	var localEndpoint *internalrelation.Endpoint
	for _, v := range eps {
		ep := v
		if ep.Name == relation.LocalEndpointName {
			localEndpoint = &ep
			break
		}
	}
	if localEndpoint == nil {
		return nil, errors.NotFoundf("relation endpoint %v", relation.LocalEndpointName)
	}

	// Add the remote application reference.
	// We construct a unique, opaque application name based on the token passed
	// in from the consuming model. This model, which is offering the
	// application being related to, does not need to know the name of the
	// consuming application.
	uniqueRemoteApplicationName := "remote-" + strings.Replace(relation.ApplicationToken, "-", "", -1)
	remoteEndpoint := internalrelation.Endpoint{
		ApplicationName: uniqueRemoteApplicationName,
		Relation: charm.Relation{
			Name:      relation.RemoteEndpoint.Name,
			Interface: relation.RemoteEndpoint.Interface,
			Role:      relation.RemoteEndpoint.Role,
		},
	}

	sourceModelTag, err := names.ParseModelTag(relation.SourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	existingRemoteApp, err := api.st.RemoteApplication(uniqueRemoteApplicationName)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		if existingRemoteApp.ConsumeVersion() < relation.ConsumeVersion {
			// TODO(wallyworld) - this operation should be in a single txn.
			api.logger.Debugf(context.TODO(), "consume version %d of remote app for offer %v: %v", relation.ConsumeVersion, relation.OfferUUID, uniqueRemoteApplicationName)
			op := existingRemoteApp.DestroyOperation(true)
			if err := api.st.ApplyOperation(op); err != nil {
				return nil, errors.Annotatef(err, "removing old saas application proxy for offer %v: %v", relation.OfferUUID, uniqueRemoteApplicationName)
			}
		}
	}

	_, err = api.st.AddRemoteApplication(commoncrossmodel.AddRemoteApplicationParams{
		Name:            uniqueRemoteApplicationName,
		SourceModel:     sourceModelTag,
		Token:           relation.ApplicationToken,
		Endpoints:       []charm.Relation{remoteEndpoint.Relation},
		IsConsumerProxy: true,
		ConsumeVersion:  relation.ConsumeVersion,
	})
	// If it already exists, that's fine.
	if err != nil && !errors.Is(err, errors.AlreadyExists) {
		return nil, errors.Annotatef(err, "adding remote application %v", uniqueRemoteApplicationName)
	}
	api.logger.Debugf(context.TODO(), "added remote application %v to local model with token %v from model %s", uniqueRemoteApplicationName, relation.ApplicationToken, sourceModelTag.Id())

	// Now add the relation if it doesn't already exist.
	localRel, err := api.st.EndpointsRelation(*localEndpoint, remoteEndpoint)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if err != nil { // not found
		localRel, err = api.st.AddRelation(*localEndpoint, remoteEndpoint)
		// Again, if it already exists, that's fine.
		if err != nil && !errors.Is(err, errors.AlreadyExists) {
			return nil, errors.Annotate(err, "adding remote relation")
		} else if err == nil {
			api.logger.Debugf(context.TODO(), "added relation %v to model %s", localRel.Tag().Id(), api.modelID)
		}
	}
	_, err = api.st.AddOfferConnection(commoncrossmodel.AddOfferConnectionParams{
		SourceModelUUID: sourceModelTag.Id(), Username: username,
		OfferUUID:   appOffer.OfferUUID,
		RelationId:  localRel.Id(),
		RelationKey: localRel.Tag().Id(),
	})
	if err != nil && !errors.Is(err, errors.AlreadyExists) {
		return nil, errors.Annotate(err, "adding offer connection details")
	}
	api.relationToOffer[localRel.Tag().Id()] = relation.OfferUUID

	// Ensure we have references recorded.
	api.logger.Debugf(context.TODO(), "importing remote relation into model %s", api.modelID)
	api.logger.Debugf(context.TODO(), "remote model is %s", sourceModelTag.Id())

	err = api.st.ImportRemoteEntity(localRel.Tag(), relation.RelationToken)
	if err != nil && !errors.Is(err, errors.AlreadyExists) {
		return nil, errors.Annotatef(err, "importing remote relation %v to local model", localRel.Tag().Id())
	}
	api.logger.Debugf(context.TODO(), "relation token %v exported for %v ", relation.RelationToken, localRel.Tag().Id())

	// Export the local offer from this model so we can tell the caller what the remote id is.
	// NB we need to export the offer last so that everything else is in place when the worker is
	// woken up by the watcher.
	token, err := api.st.ExportLocalEntity(names.NewApplicationOfferTag(appOffer.OfferUUID))
	if err != nil && !errors.Is(err, errors.AlreadyExists) {
		return nil, errors.Annotatef(err, "exporting local application offer %q", appOffer.OfferName)
	}
	api.logger.Debugf(context.TODO(), "local application offer %v from model %s exported with token %q ", appOffer.OfferName, api.modelID, token)

	// Mint a new macaroon attenuated to the actual relation.
	relationMacaroon, err := api.authCtxt.CreateRemoteRelationMacaroon(
		ctx, api.modelID, relation.OfferUUID, username, localRel.Tag(), relation.BakeryVersion)
	if err != nil {
		return nil, errors.Annotate(err, "creating relation macaroon")
	}
	return &params.RemoteRelationDetails{
		Token:    token,
		Macaroon: relationMacaroon.M(),
	}, nil
}

// WatchRelationChanges starts a RemoteRelationChangesWatcher for each
// specified relation, returning the watcher IDs and initial values,
// or an error if the remote relations couldn't be watched.
func (api *CrossModelRelationsAPIv3) WatchRelationChanges(ctx context.Context, remoteRelationArgs params.RemoteEntityArgs) (
	params.RemoteRelationWatchResults, error,
) {
	results := params.RemoteRelationWatchResults{
		Results: make([]params.RemoteRelationWatchResult, len(remoteRelationArgs.Args)),
	}

	watchOne := func(arg params.RemoteEntityArg) (common.RelationUnitsWatcher, params.RemoteRelationChangeEvent, error) {
		var empty params.RemoteRelationChangeEvent
		tag, err := api.st.GetRemoteEntity(arg.Token)
		if err != nil {
			return nil, empty, errors.Annotatef(err, "getting relation for token %q", arg.Token)
		}
		if err := api.checkMacaroonsForRelation(ctx, tag, arg.Macaroons, arg.BakeryVersion); err != nil {
			return nil, empty, errors.Trace(err)
		}
		relationTag, ok := tag.(names.RelationTag)
		if !ok {
			return nil, empty, apiservererrors.ErrPerm
		}
		relationToken, offerToken, err := commoncrossmodel.GetOfferingRelationTokens(api.st, relationTag)
		if err != nil {
			return nil, empty, errors.Annotatef(err, "getting offering relation tokens")
		}
		w, err := commoncrossmodel.WatchRelationUnits(api.st, relationTag)
		if err != nil {
			return nil, empty, errors.Annotate(err, "watching relation units")
		}
		change, ok := <-w.Changes()
		if !ok {
			return nil, empty, watcher.EnsureErr(w)
		}
		fullChange, err := commoncrossmodel.ExpandChange(api.st, relationToken, offerToken, change)
		if err != nil {
			w.Kill()
			return nil, empty, errors.Annotatef(err, "expanding relation unit change %# v", pretty.Formatter(change))
		}
		wrapped := &commoncrossmodel.WrappedUnitsWatcher{
			RelationUnitsWatcher:    w,
			RelationToken:           relationToken,
			ApplicationOrOfferToken: offerToken,
		}
		return wrapped, fullChange, nil
	}

	for i, arg := range remoteRelationArgs.Args {
		w, changes, err := watchOne(arg)
		if err != nil {
			if api.logger.IsLevelEnabled(logger.TRACE) {
				api.logger.Tracef(context.TODO(), "error watching relation for token %s: %s", arg.Token, errors.ErrorStack(err))
			} else {
				api.logger.Debugf(context.TODO(), "error watching relation for token %s: %v", err)
			}
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i].RemoteRelationWatcherId = api.resources.Register(w)
		results.Results[i].Changes = changes
	}
	return results, nil
}

func watchRelationLifeSuspendedStatus(st CrossModelRelationsState, tag names.RelationTag) (state.StringsWatcher, error) {
	relation, err := st.KeyRelation(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relation.WatchLifeSuspendedStatus(), nil
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
		relationTag, err := api.st.GetRemoteEntity(arg.Token)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(ctx, relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		w, err := api.relationStatusWatcher(api.st, relationTag.(names.RelationTag))
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		changes, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = apiservererrors.ServerError(watcher.EnsureErr(w))
			continue
		}
		changesParams := make([]params.RelationLifeSuspendedStatusChange, len(changes))
		for j, key := range changes {
			change, err := commoncrossmodel.GetRelationLifeSuspendedStatusChange(api.st, key)
			if err != nil {
				results.Results[i].Error = apiservererrors.ServerError(err)
				changesParams = nil
				_ = w.Stop()
				break
			}
			changesParams[j] = *change
		}
		results.Results[i].Changes = changesParams
		results.Results[i].RelationStatusWatcherId = api.resources.Register(w)
	}
	return results, nil
}

// OfferWatcher instances track changes to a specified offer.
type OfferWatcher interface {
	Err() error
	corewatcher.NotifyWatcher
	OfferUUID() string
	OfferName() string
}

func watchOfferStatus(ctx context.Context, st CrossModelRelationsState, offerUUID string) (OfferWatcher, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func watchConsumedSecrets(ctx context.Context, s SecretService, appName string) (corewatcher.StringsWatcher, error) {
	return s.WatchRemoteConsumedSecretsChanges(ctx, appName)
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

	auth := api.authCtxt.Authenticator()
	for i, arg := range offerArgs.Args {
		// Ensure the supplied macaroon allows access.
		_, err := auth.CheckOfferMacaroons(ctx, api.modelID, arg.OfferUUID, arg.Macaroons, arg.BakeryVersion)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := api.offerStatusWatcher(ctx, api.st, arg.OfferUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if _, err := internal.FirstResult[struct{}](ctx, w); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		change, err := commoncrossmodel.GetOfferStatusChange(ctx, api.st, api.applicationService, arg.OfferUUID, w.OfferName())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			w.Kill()
			_ = w.Wait()
			continue
		}
		results.Results[i].Changes = []params.OfferStatusChange{*change}
		results.Results[i].OfferStatusWatcherId = api.resources.Register(w)
	}
	return results, nil
}

// WatchConsumedSecretsChanges returns a watcher which notifies of changes to any secrets
// for the specified remote consumers.
func (api *CrossModelRelationsAPIv3) WatchConsumedSecretsChanges(ctx context.Context, args params.WatchRemoteSecretChangesArgs) (params.SecretRevisionWatchResults, error) {
	results := params.SecretRevisionWatchResults{
		Results: make([]params.SecretRevisionWatchResult, len(args.Args)),
	}

	auth := api.authCtxt.Authenticator()
	for i, arg := range args.Args {
		appTag, offerUUID, err := api.lookupOfferDetails(arg.ApplicationToken, arg.RelationToken)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				err = apiservererrors.ErrPerm
			}
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if offerUUID == "" {
			declared := checkers.InferDeclared(internalmacaroon.MacaroonNamespace, arg.Macaroons)
			offerUUID = declared["offer-uuid"]
		}

		// Ensure the supplied macaroon allows access.
		_, err = auth.CheckOfferMacaroons(ctx, api.modelID, offerUUID, arg.Macaroons, arg.BakeryVersion)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := api.consumedSecretsWatcher(ctx, api.secretService, appTag.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		uris, ok := <-w.Changes()
		if !ok {
			return results, apiservererrors.ServerError(worker.Stop(w))
		}
		changes, err := api.getSecretChanges(ctx, uris)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			_ = worker.Stop(w)
			continue
		}
		results.Results[i] = params.SecretRevisionWatchResult{
			WatcherId: api.resources.Register(w),
			Changes:   changes,
		}
	}
	return results, nil
}

func (api *CrossModelRelationsAPIv3) lookupOfferDetails(appToken, relToken string) (names.Tag, string, error) {
	appTag, err := api.st.GetRemoteEntity(appToken)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	// TODO(juju4) - remove
	// For compatibility with older clients which do not
	// provide a relation tag.
	if relToken == "" {
		return appTag, "", nil
	}

	relTag, err := api.st.GetRemoteEntity(relToken)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	conn, err := api.st.OfferConnectionForRelation(relTag.Id())
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return appTag, conn.OfferUUID(), nil
}

func (api *CrossModelRelationsAPIv3) getSecretChanges(ctx context.Context, uris []string) ([]params.SecretRevisionChange, error) {
	changes := make([]params.SecretRevisionChange, len(uris))
	for i, uriStr := range uris {
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		md, err := api.secretService.GetSecret(ctx, uri)
		if err != nil {
			return nil, errors.Trace(err)
		}
		changes[i] = params.SecretRevisionChange{
			URI:            uri.String(),
			LatestRevision: md.LatestRevision,
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
		relationTag, err := api.st.GetRemoteEntity(change.RelationToken)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		api.logger.Debugf(context.TODO(), "relation tag for token %+v is %v", change.RelationToken, relationTag)

		if err := api.checkMacaroonsForRelation(ctx, relationTag, change.Macaroons, change.BakeryVersion); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := commoncrossmodel.PublishIngressNetworkChange(ctx, api.modelID, api.st, api.modelConfigService, relationTag, change); err != nil {
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
	var relations params.Entities
	for i, arg := range remoteRelationArgs.Args {
		relationTag, err := api.st.GetRemoteEntity(arg.Token)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(ctx, relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		relations.Entities = append(relations.Entities, params.Entity{Tag: relationTag.String()})
	}
	watchResults, err := api.egressAddressWatcher(api.resources, api.fw, api.modelConfigService, relations)
	if err != nil {
		return results, err
	}
	index := 0
	for i, r := range results.Results {
		if r.Error != nil {
			continue
		}
		results.Results[i] = watchResults.Results[index]
		index++
	}
	return results, nil
}
