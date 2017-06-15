// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
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
	return NewCrossModelRelationsAPI(stateShim{ctx.State()}, ctx.Resources(), ctx.Auth())
}

// NewCrossModelRelationsAPI returns a new server-side CrossModelRelationsAPI facade.
func NewCrossModelRelationsAPI(
	st CrossModelRelationsState,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*CrossModelRelationsAPI, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &CrossModelRelationsAPI{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// PublishLocalRelationChange publishes local relations changes to the
// remote side offering those relations.
func (api *CrossModelRelationsAPI) PublishLocalRelationChange(
	changes params.RemoteRelationsChanges,
) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(changes.Changes)),
	}
	for i, change := range changes.Changes {
		if err := api.publishRelationChange(change); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}

func (api *CrossModelRelationsAPI) publishRelationChange(change params.RemoteRelationChangeEvent) error {
	logger.Debugf("publish into model %v change: %+v", api.st.ModelUUID(), change)

	relationTag, err := api.getRemoteEntityTag(change.RelationId)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Debugf("not found relation tag %+v in model %v, exit early", change.RelationId, api.st.ModelUUID())
			return nil
		}
		return errors.Trace(err)
	}
	logger.Debugf("relation tag for remote id %+v is %v", change.RelationId, relationTag)

	// Ensure the relation exists.
	rel, err := api.st.KeyRelation(relationTag.Id())
	if errors.IsNotFound(err) {
		if change.Life != params.Alive {
			return nil
		}
	}
	if err != nil {
		return errors.Trace(err)
	}

	// Look up the application on the remote side of this relation
	// ie from the model which published this change.
	applicationTag, err := api.getRemoteEntityTag(change.ApplicationId)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("application tag for remote id %+v is %v", change.ApplicationId, applicationTag)

	// If the remote model has destroyed the relation, do it here also.
	if change.Life != params.Alive {
		logger.Debugf("remote side of %v died", relationTag)
		if err := rel.Destroy(); err != nil {
			return errors.Trace(err)
		}
		// See if we need to remove the remote application proxy - we do this
		// on the offering side as there is 1:1 between proxy and consuming app.
		if applicationTag != nil {
			remoteApp, err := api.st.RemoteApplication(applicationTag.Id())
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			if err == nil && remoteApp.IsConsumerProxy() {
				logger.Debugf("destroy consuming app proxy for %v", applicationTag.Id())
				if err := remoteApp.Destroy(); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}

	// TODO(wallyworld) - deal with remote application being removed
	if applicationTag == nil {
		logger.Infof("no remote application found for %v", relationTag.Id())
		return nil
	}
	logger.Debugf("remote applocation for changed relation %v is %v", relationTag.Id(), applicationTag.Id())

	for _, id := range change.DepartedUnits {
		unitTag := names.NewUnitTag(fmt.Sprintf("%s/%v", applicationTag.Id(), id))
		logger.Debugf("unit %v has departed relation %v", unitTag.Id(), relationTag.Id())
		ru, err := rel.RemoteUnit(unitTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("%s leaving scope", unitTag.Id())
		if err := ru.LeaveScope(); err != nil {
			return errors.Trace(err)
		}
	}

	for _, change := range change.ChangedUnits {
		unitTag := names.NewUnitTag(fmt.Sprintf("%s/%v", applicationTag.Id(), change.UnitId))
		logger.Debugf("changed unit tag for remote id %v is %v", change.UnitId, unitTag)
		ru, err := rel.RemoteUnit(unitTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		inScope, err := ru.InScope()
		if err != nil {
			return errors.Trace(err)
		}
		settings := make(map[string]interface{})
		for k, v := range change.Settings {
			settings[k] = v
		}
		if !inScope {
			logger.Debugf("%s entering scope (%v)", unitTag.Id(), settings)
			err = ru.EnterScope(settings)
		} else {
			logger.Debugf("%s updated settings (%v)", unitTag.Id(), settings)
			err = ru.ReplaceSettings(settings)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (api *CrossModelRelationsAPI) getRemoteEntityTag(id params.RemoteEntityId) (names.Tag, error) {
	modelTag := names.NewModelTag(id.ModelUUID)
	return api.st.GetRemoteEntity(modelTag, id.Token)
}

// RegisterRemoteRelations sets up the local model to participate
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
