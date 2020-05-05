// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"strings"
	"sync"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.crossmodelrelations")

type egressAddressWatcherFunc func(facade.Resources, firewall.State, params.Entities) (params.StringsWatchResults, error)
type relationStatusWatcherFunc func(CrossModelRelationsState, names.RelationTag) (state.StringsWatcher, error)
type offerStatusWatcherFunc func(CrossModelRelationsState, string) (OfferWatcher, error)

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

	egressAddressWatcher  egressAddressWatcherFunc
	relationStatusWatcher relationStatusWatcherFunc
	offerStatusWatcher    offerStatusWatcherFunc
}

// CrossModelRelationsAPIV1 has WatchRelationUnits rather than WatchRelationChanges.
type CrossModelRelationsAPIV1 struct {
	*CrossModelRelationsAPI
}

// NewStateCrossModelRelationsAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func NewStateCrossModelRelationsAPI(ctx facade.Context) (*CrossModelRelationsAPI, error) {
	authCtxt := ctx.Resources().Get("offerAccessAuthContext").(common.ValueResource).Value
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, err
	}

	return NewCrossModelRelationsAPI(
		stateShim{
			st:      st,
			Backend: commoncrossmodel.GetBackend(st),
		},
		firewall.StateShim(st, model),
		ctx.Resources(), ctx.Auth(), authCtxt.(*commoncrossmodel.AuthContext),
		firewall.WatchEgressAddressesForRelations,
		watchRelationLifeSuspendedStatus,
		watchOfferStatus,
	)
}

// NewStateCrossModelRelationsAPIV1 creates a new server-side
// CrossModelRelations v1 API facade backed by state.
func NewStateCrossModelRelationsAPIV1(ctx facade.Context) (*CrossModelRelationsAPIV1, error) {
	api, err := NewStateCrossModelRelationsAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CrossModelRelationsAPIV1{api}, nil
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
) (*CrossModelRelationsAPI, error) {
	return &CrossModelRelationsAPI{
		ctx:                   context.Background(),
		st:                    st,
		fw:                    fw,
		resources:             resources,
		authorizer:            authorizer,
		authCtxt:              authCtxt,
		egressAddressWatcher:  egressAddressWatcher,
		relationStatusWatcher: relationStatusWatcher,
		offerStatusWatcher:    offerStatusWatcher,
		relationToOffer:       make(map[string]string),
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
	auth := api.authCtxt.Authenticator(api.st.ModelUUID(), offerUUID)
	return auth.CheckRelationMacaroons(api.ctx, relationTag, mac, version)
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		logger.Debugf("relation tag for token %+v is %v", change.RelationToken, relationTag)
		if err := api.checkMacaroonsForRelation(relationTag, change.Macaroons, change.BakeryVersion); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := commoncrossmodel.PublishRelationChange(api.st, relationTag, change); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if change.Life != life.Alive {
			delete(api.relationToOffer, relationTag.Id())
		}
	}
	return results, nil
}

// RegisterRemoteRelationArgs sets up the model to participate
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
		results.Results[i].Error = common.ServerError(err)
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
	auth := api.authCtxt.Authenticator(api.st.ModelUUID(), appOffer.OfferUUID)
	attr, err := auth.CheckOfferMacaroons(api.ctx, appOffer.OfferUUID, relation.Macaroons, relation.BakeryVersion)
	if err != nil {
		return nil, err
	}
	// The macaroon needs to be attenuated to a user.
	username, ok := attr["username"]
	if username == "" || !ok {
		return nil, common.ErrPerm
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
	for _, ep := range eps {
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
	_, err = api.st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            uniqueRemoteApplicationName,
		OfferUUID:       relation.OfferUUID,
		SourceModel:     sourceModelTag,
		Token:           relation.ApplicationToken,
		Endpoints:       []charm.Relation{remoteEndpoint.Relation},
		IsConsumerProxy: true,
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

	// Juju 2.5.1 and earlier exported the local application name so for backwards compatibility
	// use that if it's there.
	token, err := api.st.GetToken(names.NewApplicationTag(localApplicationName))
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "checking local application token for %v", localApplicationName)
	}
	if err != nil {
		// No token yet so export using the offer name which we prefer.
		token, err = api.st.ExportLocalEntity(names.NewApplicationTag(appOffer.OfferName))
		if err != nil && !errors.IsAlreadyExists(err) {
			return nil, errors.Annotatef(err, "exporting local application offer %v", appOffer.OfferName)
		}
	}
	logger.Debugf("local application offer %v from model %v exported with token %v ", appOffer.OfferName, api.st.ModelUUID(), token)

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

// WatchRelationUnits starts a RelationUnitsWatcher for watching the
// relation units involved in each specified relation, and returns the
// watcher IDs and initial values, or an error if the relation units
// could not be watched.  WatchRelationUnits is only supported on the
// v1 API - later versions provide WatchRelationChanges instead.
func (api *CrossModelRelationsAPIV1) WatchRelationUnits(remoteRelationArgs params.RemoteEntityArgs) (params.RelationUnitsWatchResults, error) {
	results := params.RelationUnitsWatchResults{
		Results: make([]params.RelationUnitsWatchResult, len(remoteRelationArgs.Args)),
	}
	for i, arg := range remoteRelationArgs.Args {
		relationTag, err := api.st.GetRemoteEntity(arg.Token)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		w, err := commoncrossmodel.WatchRelationUnits(api.st, relationTag.(names.RelationTag))
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		changes, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = common.ServerError(watcher.EnsureErr(w))
			continue
		}
		results.Results[i].RelationUnitsWatcherId = api.resources.Register(w)
		results.Results[i].Changes = changes
	}
	return results, nil
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
			return nil, empty, errors.Trace(err)
		}
		if err := api.checkMacaroonsForRelation(tag, arg.Macaroons, arg.BakeryVersion); err != nil {
			return nil, empty, errors.Trace(err)
		}
		relationTag, ok := tag.(names.RelationTag)
		if !ok {
			return nil, empty, common.ErrPerm
		}
		relationToken, appToken, err := commoncrossmodel.GetOfferingRelationTokens(api.st, relationTag)
		if err != nil {
			return nil, empty, errors.Trace(err)
		}
		w, err := commoncrossmodel.WatchRelationUnits(api.st, relationTag)
		if err != nil {
			return nil, empty, errors.Trace(err)
		}
		change, ok := <-w.Changes()
		if !ok {
			return nil, empty, watcher.EnsureErr(w)
		}
		fullChange, err := commoncrossmodel.ExpandChange(api.st, relationToken, appToken, change)
		if err != nil {
			w.Kill()
			return nil, empty, errors.Trace(err)
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
			logger.Tracef("not found watching relation %s: %s", arg.Token, errors.ErrorStack(err))
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		results.Results[i].RemoteRelationWatcherId = api.resources.Register(w)
		results.Results[i].Changes = changes
	}
	return results, nil
}

// Mask out new methods from the old API versions. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is concerned.
//
// WatchRelationChanges doesn't exist before the v2 API.
func (api *CrossModelRelationsAPIV1) WatchRelationChanges(_, _ struct{}) {}

// RelationUnitSettings returns the relation unit settings for the
// given relation units. (Removed in v2 of the API, the events
// returned by WatchRelationChanges include the full settings.)
func (api *CrossModelRelationsAPIV1) RelationUnitSettings(relationUnits params.RemoteRelationUnits) (params.SettingsResults, error) {
	results := params.SettingsResults{
		Results: make([]params.SettingsResult, len(relationUnits.RelationUnits)),
	}
	for i, arg := range relationUnits.RelationUnits {
		relationTag, err := api.st.GetRemoteEntity(arg.RelationToken)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		ru := params.RelationUnit{
			Relation: relationTag.String(),
			Unit:     arg.Unit,
		}
		settings, err := commoncrossmodel.RelationUnitSettings(api.st, ru)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Settings = settings
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		w, err := api.relationStatusWatcher(api.st, relationTag.(names.RelationTag))
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		changes, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = common.ServerError(watcher.EnsureErr(w))
			continue
		}
		changesParams := make([]params.RelationLifeSuspendedStatusChange, len(changes))
		for j, key := range changes {
			change, err := commoncrossmodel.GetRelationLifeSuspendedStatusChange(api.st, key)
			if err != nil {
				results.Results[i].Error = common.ServerError(err)
				changesParams = nil
				w.Stop()
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
}

type offerWatcher struct {
	state.NotifyWatcher
	offerUUID string
}

func (w *offerWatcher) OfferUUID() string {
	return w.offerUUID
}

func watchOfferStatus(st CrossModelRelationsState, offerUUID string) (OfferWatcher, error) {
	w, err := st.WatchOfferStatus(offerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &offerWatcher{w, offerUUID}, nil
}

// WatchOfferStatus starts an OfferStatusWatcher for
// watching the status of an offer.
func (api *CrossModelRelationsAPI) WatchOfferStatus(
	offerArgs params.OfferArgs,
) (params.OfferStatusWatchResults, error) {
	results := params.OfferStatusWatchResults{
		Results: make([]params.OfferStatusWatchResult, len(offerArgs.Args)),
	}

	for i, arg := range offerArgs.Args {
		// Ensure the supplied macaroon allows access.
		auth := api.authCtxt.Authenticator(api.st.ModelUUID(), arg.OfferUUID)
		_, err := auth.CheckOfferMacaroons(api.ctx, arg.OfferUUID, arg.Macaroons, arg.BakeryVersion)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		w, err := api.offerStatusWatcher(api.st, arg.OfferUUID)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		_, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = common.ServerError(watcher.EnsureErr(w))
			continue
		}
		change, err := commoncrossmodel.GetOfferStatusChange(api.st, arg.OfferUUID)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			w.Stop()
			break
		}
		results.Results[i].Changes = []params.OfferStatusChange{*change}
		results.Results[i].OfferStatusWatcherId = api.resources.Register(w)
	}
	return results, nil
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		logger.Debugf("relation tag for token %+v is %v", change.RelationToken, relationTag)

		if err := api.checkMacaroonsForRelation(relationTag, change.Macaroons, change.BakeryVersion); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := commoncrossmodel.PublishIngressNetworkChange(api.st, relationTag, change); err != nil {
			results.Results[i].Error = common.ServerError(err)
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons, arg.BakeryVersion); err != nil {
			results.Results[i].Error = common.ServerError(err)
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
