// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"strings"
	"sync"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/kr/pretty"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	coremacaroon "github.com/juju/juju/core/macaroon"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLoggerWithLabels("juju.apiserver.crossmodelrelations", corelogger.CMR)

type egressAddressWatcherFunc func(facade.Resources, firewall.State, params.Entities) (params.StringsWatchResults, error)
type relationStatusWatcherFunc func(CrossModelRelationsState, names.RelationTag) (state.StringsWatcher, error)
type offerStatusWatcherFunc func(CrossModelRelationsState, string) (OfferWatcher, error)
type consumedSecretsWatcherFunc func(CrossModelRelationsState, string) (state.StringsWatcher, error)

// CrossModelRelationsAPI provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPI struct {
	ctx        context.Context
	st         CrossModelRelationsState
	fw         firewall.State
	resources  facade.Resources
	authorizer facade.Authorizer

	mu              sync.Mutex
	authCtxt        *commoncrossmodel.AuthContext
	relationToOffer map[string]string

	egressAddressWatcher   egressAddressWatcherFunc
	relationStatusWatcher  relationStatusWatcherFunc
	offerStatusWatcher     offerStatusWatcherFunc
	consumedSecretsWatcher consumedSecretsWatcherFunc
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI(
	st CrossModelRelationsState,
	fw firewall.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	authCtxt *commoncrossmodel.AuthContext,
	egressAddressWatcher egressAddressWatcherFunc,
	relationStatusWatcher relationStatusWatcherFunc,
	offerStatusWatcher offerStatusWatcherFunc,
	consumedSecretsWatcher consumedSecretsWatcherFunc,
) (*CrossModelRelationsAPI, error) {
	return &CrossModelRelationsAPI{
		ctx:                    context.Background(),
		st:                     st,
		fw:                     fw,
		resources:              resources,
		authorizer:             authorizer,
		authCtxt:               authCtxt,
		egressAddressWatcher:   egressAddressWatcher,
		relationStatusWatcher:  relationStatusWatcher,
		offerStatusWatcher:     offerStatusWatcher,
		consumedSecretsWatcher: consumedSecretsWatcher,
		relationToOffer:        make(map[string]string),
	}, nil
}

func (api *CrossModelRelationsAPI) checkMacaroonsForRelation(relationTag names.Tag, mac macaroon.Slice, version bakery.Version) error {
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
	return auth.CheckRelationMacaroons(api.ctx, api.st.ModelUUID(), offerUUID, relationTag, mac, version)
}

// PublishRelationChanges publishes relation changes to the
// model hosting the remote application involved in the relation.
func (api *CrossModelRelationsAPI) PublishRelationChanges(
	changes params.RemoteRelationsChanges,
) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(changes.Changes)),
	}
	for i, change := range changes.Changes {
		relationTag, err := api.st.GetRemoteEntity(change.RelationToken)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Debugf("no relation tag %+v in model %v, exit early", change.RelationToken, api.st.ModelUUID())
				continue
			}
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		logger.Debugf("relation tag for token %+v is %v", change.RelationToken, relationTag)
		if err := api.checkMacaroonsForRelation(relationTag, change.Macaroons, change.BakeryVersion); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Look up the application on the remote side of this relation
		// ie from the model which published this change.
		appOrOfferTag, err := api.st.GetRemoteEntity(change.ApplicationToken)
		if err != nil && !errors.IsNotFound(err) {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// The tag is either an application tag (legacy), or an offer tag.
		var applicationTag names.Tag
		if err == nil {
			switch k := appOrOfferTag.Kind(); k {
			case names.ApplicationTagKind:
				applicationTag = appOrOfferTag
			case names.ApplicationOfferTagKind:
				// For an offer tag, load the offer and get the offered app from that.
				appName, err := api.st.AppNameForOffer(appOrOfferTag.Id())
				if err != nil && !errors.IsNotFound(err) {
					results.Results[i].Error = apiservererrors.ServerError(err)
					continue
				}
				if err == nil {
					applicationTag = names.NewApplicationTag(appName)
				}
			default:
				// Should never happen.
				results.Results[i].Error = apiservererrors.ServerError(errors.NotValidf("offer app tag kind %q", k))
				continue
			}
		}

		if err := commoncrossmodel.PublishRelationChange(api.authorizer, api.st, relationTag, applicationTag, change); err != nil {
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
func (api *CrossModelRelationsAPI) RegisterRemoteRelations(
	relations params.RegisterRemoteRelationArgs,
) (params.RegisterRemoteRelationResults, error) {
	results := params.RegisterRemoteRelationResults{
		Results: make([]params.RegisterRemoteRelationResult, len(relations.Relations)),
	}
	for i, relation := range relations.Relations {
		id, err := api.registerRemoteRelation(relation)
		results.Results[i].Result = id
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (api *CrossModelRelationsAPI) registerRemoteRelation(relation params.RegisterRemoteRelationArg) (*params.RemoteRelationDetails, error) {
	logger.Debugf("register remote relation %+v", relation)
	// TODO(wallyworld) - do this as a transaction so the result is atomic
	// Perform some initial validation - is the local application alive?

	// Look up the offer record so get the local application to which we need to relate.
	appOffer, err := api.st.ApplicationOfferForUUID(relation.OfferUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Check that the supplied macaroon allows access.
	auth := api.authCtxt.Authenticator()
	attr, err := auth.CheckOfferMacaroons(api.ctx, api.st.ModelUUID(), appOffer.OfferUUID, relation.Macaroons, relation.BakeryVersion)
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
		logger.Warningf("local application for offer %v not found", localApplicationName)
		return nil, errors.NotFoundf("local application for offer %v", relation.OfferUUID)
	}
	eps, err := localApp.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Does the requested local endpoint exist?
	var localEndpoint *state.Endpoint
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
	remoteEndpoint := state.Endpoint{
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
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		if existingRemoteApp.ConsumeVersion() < relation.ConsumeVersion {
			// TODO(wallyworld) - this operation should be in a single txn.
			logger.Debugf("consume version %d of remote app for offer %v: %v", relation.ConsumeVersion, relation.OfferUUID, uniqueRemoteApplicationName)
			op := existingRemoteApp.DestroyOperation(true)
			if err := api.st.ApplyOperation(op); err != nil {
				return nil, errors.Annotatef(err, "removing old saas application proxy for offer %v: %v", relation.OfferUUID, uniqueRemoteApplicationName)
			}
		}
	}

	_, err = api.st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            uniqueRemoteApplicationName,
		SourceModel:     sourceModelTag,
		Token:           relation.ApplicationToken,
		Endpoints:       []charm.Relation{remoteEndpoint.Relation},
		IsConsumerProxy: true,
		ConsumeVersion:  relation.ConsumeVersion,
	})
	// If it already exists, that's fine.
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotatef(err, "adding remote application %v", uniqueRemoteApplicationName)
	}
	logger.Debugf("added remote application %v to local model with token %v from model %v", uniqueRemoteApplicationName, relation.ApplicationToken, sourceModelTag.Id())

	// Now add the relation if it doesn't already exist.
	localRel, err := api.st.EndpointsRelation(*localEndpoint, remoteEndpoint)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err != nil { // not found
		localRel, err = api.st.AddRelation(*localEndpoint, remoteEndpoint)
		// Again, if it already exists, that's fine.
		if err != nil && !errors.IsAlreadyExists(err) {
			return nil, errors.Annotate(err, "adding remote relation")
		}
		logger.Debugf("added relation %v to model %v", localRel.Tag().Id(), api.st.ModelUUID())
	}
	_, err = api.st.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: sourceModelTag.Id(), Username: username,
		OfferUUID:   appOffer.OfferUUID,
		RelationId:  localRel.Id(),
		RelationKey: localRel.Tag().Id(),
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotate(err, "adding offer connection details")
	}
	api.relationToOffer[localRel.Tag().Id()] = relation.OfferUUID

	// Ensure we have references recorded.
	logger.Debugf("importing remote relation into model %v", api.st.ModelUUID())
	logger.Debugf("remote model is %v", sourceModelTag.Id())

	err = api.st.ImportRemoteEntity(localRel.Tag(), relation.RelationToken)
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotatef(err, "importing remote relation %v to local model", localRel.Tag().Id())
	}
	logger.Debugf("relation token %v exported for %v ", relation.RelationToken, localRel.Tag().Id())

	// Export the local offer from this model so we can tell the caller what the remote id is.
	// The offer is exported as an application name since it models the behaviour of an application
	// as far as the consuming side is concerned, and also needs to be unique.
	// This allows > 1 offers off the one application to be made.
	// NB we need to export the application last so that everything else is in place when the worker is
	// woken up by the watcher.
	token, err := api.st.ExportLocalEntity(names.NewApplicationOfferTag(appOffer.OfferName))
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotatef(err, "exporting local application offer %q", appOffer.OfferName)
	}
	logger.Debugf("local application offer %v from model %v exported with token %q ", appOffer.OfferName, api.st.ModelUUID(), token)

	// Mint a new macaroon attenuated to the actual relation.
	relationMacaroon, err := api.authCtxt.CreateRemoteRelationMacaroon(
		api.ctx, api.st.ModelUUID(), relation.OfferUUID, username, localRel.Tag(), relation.BakeryVersion)
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
func (api *CrossModelRelationsAPI) WatchRelationChanges(remoteRelationArgs params.RemoteEntityArgs) (
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
		if err := api.checkMacaroonsForRelation(tag, arg.Macaroons, arg.BakeryVersion); err != nil {
			return nil, empty, errors.Trace(err)
		}
		relationTag, ok := tag.(names.RelationTag)
		if !ok {
			return nil, empty, apiservererrors.ErrPerm
		}
		relationToken, appToken, err := commoncrossmodel.GetOfferingRelationTokens(api.st, relationTag)
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
		fullChange, err := commoncrossmodel.ExpandChange(api.st, relationToken, appToken, change)
		if err != nil {
			w.Kill()
			return nil, empty, errors.Annotatef(err, "expanding relation unit change %# v", pretty.Formatter(change))
		}
		wrapped := &commoncrossmodel.WrappedUnitsWatcher{
			RelationUnitsWatcher: w,
			RelationToken:        relationToken,
			ApplicationToken:     appToken,
		}
		return wrapped, fullChange, nil
	}

	for i, arg := range remoteRelationArgs.Args {
		w, changes, err := watchOne(arg)
		if err != nil {
			if logger.IsTraceEnabled() {
				logger.Tracef("error watching relation for token %s: %s", arg.Token, errors.ErrorStack(err))
			} else {
				logger.Debugf("error watching relation for token %s: %v", err)
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
func (api *CrossModelRelationsAPI) WatchRelationsSuspendedStatus(
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
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
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
	state.NotifyWatcher
	OfferUUID() string
	OfferName() string
}

type offerWatcher struct {
	state.NotifyWatcher
	offerUUID string
	offerName string
}

func (w *offerWatcher) OfferUUID() string {
	return w.offerUUID
}

func (w *offerWatcher) OfferName() string {
	return w.offerName
}

func watchOfferStatus(st CrossModelRelationsState, offerUUID string) (OfferWatcher, error) {
	w1, err := st.WatchOfferStatus(offerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	offer, err := st.ApplicationOfferForUUID(offerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w2 := st.WatchOffer(offer.OfferName)
	mw := common.NewMultiNotifyWatcher(w1, w2)
	return &offerWatcher{mw, offerUUID, offer.OfferName}, nil
}

func watchConsumedSecrets(st CrossModelRelationsState, appName string) (state.StringsWatcher, error) {
	return st.WatchConsumedSecretsChanges(appName)
}

// WatchOfferStatus starts an OfferStatusWatcher for
// watching the status of an offer.
func (api *CrossModelRelationsAPI) WatchOfferStatus(
	offerArgs params.OfferArgs,
) (params.OfferStatusWatchResults, error) {
	results := params.OfferStatusWatchResults{
		Results: make([]params.OfferStatusWatchResult, len(offerArgs.Args)),
	}

	auth := api.authCtxt.Authenticator()
	for i, arg := range offerArgs.Args {
		// Ensure the supplied macaroon allows access.
		_, err := auth.CheckOfferMacaroons(api.ctx, api.st.ModelUUID(), arg.OfferUUID, arg.Macaroons, arg.BakeryVersion)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := api.offerStatusWatcher(api.st, arg.OfferUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		_, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = apiservererrors.ServerError(watcher.EnsureErr(w))
			continue
		}
		change, err := commoncrossmodel.GetOfferStatusChange(api.st, arg.OfferUUID, w.OfferName())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			_ = w.Stop()
			continue
		}
		results.Results[i].Changes = []params.OfferStatusChange{*change}
		results.Results[i].OfferStatusWatcherId = api.resources.Register(w)
	}
	return results, nil
}

// WatchConsumedSecretsChanges returns a watcher which notifies of changes to any secrets
// for the specified remote consumers.
func (api *CrossModelRelationsAPI) WatchConsumedSecretsChanges(args params.WatchRemoteSecretChangesArgs) (params.SecretRevisionWatchResults, error) {
	results := params.SecretRevisionWatchResults{
		Results: make([]params.SecretRevisionWatchResult, len(args.Args)),
	}

	auth := api.authCtxt.Authenticator()
	for i, arg := range args.Args {
		appTag, offerUUID, err := api.st.GetSecretConsumerInfo(arg.ApplicationToken, arg.RelationToken)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				err = apiservererrors.ErrPerm
			}
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if offerUUID == "" {
			declared := checkers.InferDeclared(coremacaroon.MacaroonNamespace, arg.Macaroons)
			offerUUID = declared["offer-uuid"]
		}

		// Ensure the supplied macaroon allows access.
		_, err = auth.CheckOfferMacaroons(api.ctx, api.st.ModelUUID(), offerUUID, arg.Macaroons, arg.BakeryVersion)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := api.consumedSecretsWatcher(api.st, appTag.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		uris, ok := <-w.Changes()
		if !ok {
			return results, apiservererrors.ServerError(watcher.EnsureErr(w))
		}
		changes, err := api.getSecretChanges(uris)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			_ = w.Stop()
			continue
		}
		results.Results[i] = params.SecretRevisionWatchResult{
			WatcherId: api.resources.Register(w),
			Changes:   changes,
		}
	}
	return results, nil
}

func (api *CrossModelRelationsAPI) getSecretChanges(uris []string) ([]params.SecretRevisionChange, error) {
	changes := make([]params.SecretRevisionChange, len(uris))
	for i, uriStr := range uris {
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		md, err := api.st.GetSecret(uri)
		if err != nil {
			return nil, errors.Trace(err)
		}
		changes[i] = params.SecretRevisionChange{
			URI:      uri.String(),
			Revision: md.LatestRevision,
		}
	}
	return changes, nil
}

// PublishIngressNetworkChanges publishes changes to the required
// ingress addresses to the model hosting the offer in the relation.
func (api *CrossModelRelationsAPI) PublishIngressNetworkChanges(
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
		logger.Debugf("relation tag for token %+v is %v", change.RelationToken, relationTag)

		if err := api.checkMacaroonsForRelation(relationTag, change.Macaroons, change.BakeryVersion); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := commoncrossmodel.PublishIngressNetworkChange(api.st, relationTag, change); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}

// WatchEgressAddressesForRelations creates a watcher that notifies when addresses, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func (api *CrossModelRelationsAPI) WatchEgressAddressesForRelations(remoteRelationArgs params.RemoteEntityArgs) (params.StringsWatchResults, error) {
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
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		relations.Entities = append(relations.Entities, params.Entity{Tag: relationTag.String()})
	}
	watchResults, err := api.egressAddressWatcher(api.resources, api.fw, relations)
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
