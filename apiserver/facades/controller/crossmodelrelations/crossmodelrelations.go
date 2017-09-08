// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.crossmodelrelations")

type egressAddressWatcherFunc func(facade.Resources, firewall.State, params.Entities) (params.StringsWatchResults, error)
type relationStatusWatcherFunc func(CrossModelRelationsState, names.RelationTag) (state.StringsWatcher, error)

// CrossModelRelationsAPI provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPI struct {
	st         CrossModelRelationsState
	fw         firewall.State
	resources  facade.Resources
	authorizer facade.Authorizer

	mu              sync.Mutex
	authCtxt        *commoncrossmodel.AuthContext
	relationToOffer map[string]string

	egressAddressWatcher  egressAddressWatcherFunc
	relationStatusWatcher relationStatusWatcherFunc
}

// NewStateCrossModelRelationsAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func NewStateCrossModelRelationsAPI(ctx facade.Context) (*CrossModelRelationsAPI, error) {
	authCtxt := ctx.Resources().Get("offerAccessAuthContext").(common.ValueResource).Value

	return NewCrossModelRelationsAPI(
		stateShim{
			st:      ctx.State(),
			Backend: commoncrossmodel.GetBackend(ctx.State()),
		},
		firewall.StateShim(ctx.State()),
		ctx.Resources(), ctx.Auth(), authCtxt.(*commoncrossmodel.AuthContext),
		firewall.WatchEgressAddressesForRelations,
		watchRelationLifeStatus,
	)
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
) (*CrossModelRelationsAPI, error) {
	return &CrossModelRelationsAPI{
		st:                    st,
		fw:                    fw,
		resources:             resources,
		authorizer:            authorizer,
		authCtxt:              authCtxt,
		egressAddressWatcher:  egressAddressWatcher,
		relationStatusWatcher: relationStatusWatcher,
		relationToOffer:       make(map[string]string),
	}, nil
}

func (api *CrossModelRelationsAPI) checkMacaroonsForRelation(relationTag names.Tag, mac macaroon.Slice) error {
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
	return auth.CheckRelationMacaroons(relationTag, mac)
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
		if err := api.checkMacaroonsForRelation(relationTag, change.Macaroons); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := commoncrossmodel.PublishRelationChange(api.st, relationTag, change); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if change.Life != params.Alive {
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
	attr, err := auth.CheckOfferMacaroons(appOffer.OfferUUID, relation.Macaroons)
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

	// Add the remote application reference. We construct a unique, opaque application name based on the
	// token passed in from the consuming model. This model, which is offering the application being
	// related to, does not need to know the name of the consuming application.
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

	// Export the local application from this model so we can tell the caller what the remote id is.
	// NB we need to export the application last so that everything else is in place when the worker is
	// woken up by the watcher.
	token, err := api.st.ExportLocalEntity(names.NewApplicationTag(localApplicationName))
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotatef(err, "exporting local application %v", localApplicationName)
	}
	logger.Debugf("local application %v from model %v exported with token %v ", localApplicationName, api.st.ModelUUID(), token)

	// Mint a new macaroon attenuated to the actual relation.
	relationMacaroon, err := api.authCtxt.CreateRemoteRelationMacaroon(
		api.st.ModelUUID(), relation.OfferUUID, username, localRel.Tag())
	if err != nil {
		return nil, errors.Annotate(err, "creating relation macaroon")
	}
	return &params.RemoteRelationDetails{
		Token:    token,
		Macaroon: relationMacaroon,
	}, nil
}

// WatchRelationUnits starts a RelationUnitsWatcher for watching the
// relation units involved in each specified relation, and returns the
// watcher IDs and initial values, or an error if the relation units could not be watched.
func (api *CrossModelRelationsAPI) WatchRelationUnits(remoteRelationArgs params.RemoteEntityArgs) (params.RelationUnitsWatchResults, error) {
	results := params.RelationUnitsWatchResults{
		Results: make([]params.RelationUnitsWatchResult, len(remoteRelationArgs.Args)),
	}
	for i, arg := range remoteRelationArgs.Args {
		relationTag, err := api.st.GetRemoteEntity(arg.Token)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons); err != nil {
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

// RelationUnitSettings returns the relation unit settings for the given relation units.
func (api *CrossModelRelationsAPI) RelationUnitSettings(relationUnits params.RemoteRelationUnits) (params.SettingsResults, error) {
	results := params.SettingsResults{
		Results: make([]params.SettingsResult, len(relationUnits.RelationUnits)),
	}
	for i, arg := range relationUnits.RelationUnits {
		relationTag, err := api.st.GetRemoteEntity(arg.RelationToken)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons); err != nil {
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

func watchRelationLifeStatus(st CrossModelRelationsState, tag names.RelationTag) (state.StringsWatcher, error) {
	relation, err := st.KeyRelation(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relation.WatchLifeStatus(), nil
}

// WatchRelationsStatus starts a RelationStatusWatcher for
// watching the life and status of a relation.
func (api *CrossModelRelationsAPI) WatchRelationsStatus(
	remoteRelationArgs params.RemoteEntityArgs,
) (params.RelationStatusWatchResults, error) {
	results := params.RelationStatusWatchResults{
		Results: make([]params.RelationStatusWatchResult, len(remoteRelationArgs.Args)),
	}

	for i, arg := range remoteRelationArgs.Args {
		relationTag, err := api.st.GetRemoteEntity(arg.Token)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons); err != nil {
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
		results.Results[i].RelationStatusWatcherId = api.resources.Register(w)
		changesParams := make([]params.RelationStatusChange, len(changes))
		for j, key := range changes {
			change, err := commoncrossmodel.GetRelationStatusChange(api.st, key)
			if err != nil {
				results.Results[i].Error = common.ServerError(err)
				changesParams = nil
				break
			}
			changesParams[j] = *change
		}
		results.Results[i].Changes = changesParams
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

		if err := api.checkMacaroonsForRelation(relationTag, change.Macaroons); err != nil {
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
		if err := api.checkMacaroonsForRelation(relationTag, arg.Macaroons); err != nil {
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
