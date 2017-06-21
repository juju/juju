// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.crossmodelrelations")

// CrossModelRelationsAPI provides access to the CrossModelRelations API facade.
type CrossModelRelationsAPI struct {
	st         CrossModelRelationsState
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewStateCrossModelRelationsAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func NewStateCrossModelRelationsAPI(ctx facade.Context) (*CrossModelRelationsAPI, error) {
	return NewCrossModelRelationsAPI(
		stateShim{st: ctx.State(), Backend: commoncrossmodel.GetBackend(ctx.State())},
		ctx.Resources(), ctx.Auth(),
	)
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI(
	st CrossModelRelationsState,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*CrossModelRelationsAPI, error) {
	// TODO(wallyworld) - auth based on macaroons.
	return &CrossModelRelationsAPI{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// PublishRelationChange publishes relation changes to the
// model hosting the remote application involved in the relation.
func (api *CrossModelRelationsAPI) PublishRelationChange(
	changes params.RemoteRelationsChanges,
) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(changes.Changes)),
	}
	for i, change := range changes.Changes {
		if err := commoncrossmodel.PublishRelationChange(api.st, change); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}

// RegisterRemoteRelations sets up the model to participate
// in the specified relations. This operation is idempotent.
func (api *CrossModelRelationsAPI) RegisterRemoteRelations(
	relations params.RegisterRemoteRelations,
) (params.RemoteEntityIdResults, error) {
	results := params.RemoteEntityIdResults{
		Results: make([]params.RemoteEntityIdResult, len(relations.Relations)),
	}
	for i, relation := range relations.Relations {
		// TODO(wallyworld) - check macaroon
		if id, err := api.registerRemoteRelation(relation); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		} else {
			results.Results[i].Result = id
		}
	}
	return results, nil
}

func (api *CrossModelRelationsAPI) registerRemoteRelation(relation params.RegisterRemoteRelation) (*params.RemoteEntityId, error) {
	logger.Debugf("register remote relation %+v", relation)
	// TODO(wallyworld) - do this as a transaction so the result is atomic
	// Perform some initial validation - is the local application alive?

	// Look up the offer record so get the local application to which we need to relate.
	appOffers, err := api.st.ListOffers(crossmodel.ApplicationOfferFilter{
		OfferName: relation.OfferName,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(appOffers) == 0 {
		return nil, errors.NotFoundf("application offer %v", relation.OfferName)
	}
	localApplicationName := appOffers[0].ApplicationName

	localApp, err := api.st.Application(localApplicationName)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get application for offer %q", relation.OfferName)
	}
	if localApp.Life() != state.Alive {
		// We don't want to leak the application name so just log it.
		logger.Warningf("local application for offer %v not found", localApplicationName)
		return nil, errors.NotFoundf("local application for offer %v", relation.OfferName)
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
	uniqueRemoteApplicationName := "remote-" + strings.Replace(relation.ApplicationId.Token, "-", "", -1)
	remoteEndpoint := state.Endpoint{
		ApplicationName: uniqueRemoteApplicationName,
		Relation: charm.Relation{
			Name:      relation.RemoteEndpoint.Name,
			Scope:     relation.RemoteEndpoint.Scope,
			Interface: relation.RemoteEndpoint.Interface,
			Role:      relation.RemoteEndpoint.Role,
			Limit:     relation.RemoteEndpoint.Limit,
		},
	}

	remoteModelTag := names.NewModelTag(relation.ApplicationId.ModelUUID)
	_, err = api.st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            uniqueRemoteApplicationName,
		OfferName:       relation.OfferName,
		SourceModel:     names.NewModelTag(relation.ApplicationId.ModelUUID),
		Token:           relation.ApplicationId.Token,
		Endpoints:       []charm.Relation{remoteEndpoint.Relation},
		IsConsumerProxy: true,
	})
	// If it already exists, that's fine.
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotatef(err, "adding remote application %v", uniqueRemoteApplicationName)
	}
	logger.Debugf("added remote application %v to local model with token %v", uniqueRemoteApplicationName, relation.ApplicationId.Token)

	// Now add the relation if it doesn't already exist.
	localRel, err := api.st.EndpointsRelation(*localEndpoint, remoteEndpoint)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err != nil {
		localRel, err = api.st.AddRelation(*localEndpoint, remoteEndpoint)
		// Again, if it already exists, that's fine.
		if err != nil && !errors.IsAlreadyExists(err) {
			return nil, errors.Annotate(err, "adding remote relation")
		}
		logger.Debugf("added relation %v to model %v", localRel.Tag().Id(), api.st.ModelUUID())
	}

	// Ensure we have references recorded.
	logger.Debugf("importing remote relation into model %v", api.st.ModelUUID())
	logger.Debugf("remote model is %v", remoteModelTag.Id())

	err = api.st.ImportRemoteEntity(remoteModelTag, localRel.Tag(), relation.RelationId.Token)
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotatef(err, "importing remote relation %v to local model", localRel.Tag().Id())
	}
	logger.Debugf("relation token %v exported for %v ", relation.RelationId.Token, localRel.Tag().Id())

	// Export the local application from this model so we can tell the caller what the remote id is.
	// NB we need to export the application last so that everything else is in place when the worker is
	// woken up by the watcher.
	token, err := api.st.ExportLocalEntity(names.NewApplicationTag(localApplicationName))
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotatef(err, "exporting local application %v", localApplicationName)
	}
	logger.Debugf("local application %v from model %v exported with token %v ", localApplicationName, api.st.ModelUUID(), token)
	return &params.RemoteEntityId{
		ModelUUID: api.st.ModelUUID(),
		Token:     token,
	}, nil
}

// WatchRelationUnits starts a RelationUnitsWatcher for watching the
// relation units involved in each specified relation, and returns the
// watcher IDs and initial values, or an error if the relation units could not be watched.
func (api *CrossModelRelationsAPI) WatchRelationUnits(remoteEntities params.RemoteEntities) (params.RelationUnitsWatchResults, error) {
	results := params.RelationUnitsWatchResults{
		Results: make([]params.RelationUnitsWatchResult, len(remoteEntities.Entities)),
	}
	for i, arg := range remoteEntities.Entities {
		relationTag, err := api.st.GetRemoteEntity(names.NewModelTag(arg.ModelUUID), arg.Token)
		if err != nil {
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
		relationTag, err := api.st.GetRemoteEntity(names.NewModelTag(arg.RelationId.ModelUUID), arg.RelationId.Token)
		if err != nil {
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
