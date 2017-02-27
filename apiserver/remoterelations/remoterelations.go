// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

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
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.remoterelations")

func init() {
	common.RegisterStandardFacadeForFeature("RemoteRelations", 1, NewStateRemoteRelationsAPI, feature.CrossModelRelations)
}

// RemoteRelationsAPI provides access to the Provisioner API facade.
type RemoteRelationsAPI struct {
	st         RemoteRelationsState
	pool       StatePool
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewRemoteRelationsAPI creates a new server-side RemoteRelationsAPI facade
// backed by global state.
func NewStateRemoteRelationsAPI(ctx facade.Context) (*RemoteRelationsAPI, error) {
	return NewRemoteRelationsAPI(stateShim{ctx.State()}, statePoolShim{ctx.StatePool()}, ctx.Resources(), ctx.Auth())
}

// NewRemoteRelationsAPI returns a new server-side RemoteRelationsAPI facade.
func NewRemoteRelationsAPI(
	st RemoteRelationsState,
	pool StatePool,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*RemoteRelationsAPI, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &RemoteRelationsAPI{
		st:         st,
		pool:       pool,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// ImportRemoteEntities adds entities to the remote entities collection with the specified opaque tokens.
func (api *RemoteRelationsAPI) ImportRemoteEntities(args params.ImportEntityArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := api.importRemoteEntity(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (api *RemoteRelationsAPI) importRemoteEntity(arg params.ImportEntityArg) error {
	entityTag, err := names.ParseTag(arg.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	modelTag, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	return api.st.ImportRemoteEntity(modelTag, entityTag, arg.Token)
}

// ExportEntities allocates unique, remote entity IDs for the given entities in the local model.
func (api *RemoteRelationsAPI) ExportEntities(entities params.Entities) (params.RemoteEntityIdResults, error) {
	results := params.RemoteEntityIdResults{
		Results: make([]params.RemoteEntityIdResult, len(entities.Entities)),
	}
	for i, entity := range entities.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		token, err := api.st.ExportLocalEntity(tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			if !errors.IsAlreadyExists(err) {
				continue
			}
		}
		results.Results[i].Result = &params.RemoteEntityId{
			ModelUUID: api.st.ModelUUID(),
			Token:     token,
		}
	}
	return results, nil
}

// GetToken returns the token associated with the entity with the given tag for the current model.
func (api *RemoteRelationsAPI) GetTokens(args params.GetTokenArgs) (params.StringResults, error) {
	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		entityTag, err := names.ParseTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		modelTag, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		token, err := api.st.GetToken(modelTag, entityTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
		results.Results[i].Result = token
	}
	return results, nil
}

// RelationUnitSettings returns the relation unit settings for the given relation units in the local model.
func (api *RemoteRelationsAPI) RelationUnitSettings(relationUnits params.RelationUnits) (params.SettingsResults, error) {
	results := params.SettingsResults{
		Results: make([]params.SettingsResult, len(relationUnits.RelationUnits)),
	}
	one := func(ru params.RelationUnit) (params.Settings, error) {
		relationTag, err := names.ParseRelationTag(ru.Relation)
		if err != nil {
			return nil, errors.Trace(err)
		}
		rel, err := api.st.KeyRelation(relationTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		unitTag, err := names.ParseUnitTag(ru.Unit)
		if err != nil {
			return nil, errors.Trace(err)
		}
		unit, err := rel.Unit(unitTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		settings, err := unit.Settings()
		if err != nil {
			return nil, errors.Trace(err)
		}
		paramsSettings := make(params.Settings)
		for k, v := range settings {
			vString, ok := v.(string)
			if !ok {
				return nil, errors.Errorf(
					"invalid relation setting %q: expected string, got %T", k, v,
				)
			}
			paramsSettings[k] = vString
		}
		return paramsSettings, nil
	}
	for i, ru := range relationUnits.RelationUnits {
		settings, err := one(ru)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Settings = settings
	}
	return results, nil
}

func (api *RemoteRelationsAPI) remoteRelation(entity params.Entity) (*params.RemoteRelation, error) {
	tag, err := names.ParseRelationTag(entity.Tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rel, err := api.st.KeyRelation(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &params.RemoteRelation{
		Id:   rel.Id(),
		Life: params.Life(rel.Life().String()),
		Key:  tag.Id(),
	}
	for _, ep := range rel.Endpoints() {
		// Try looking up the info for the remote application.
		remoteApp, err := api.st.RemoteApplication(ep.ApplicationName)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		} else if err == nil {
			result.RemoteEndpointName = ep.Name
			result.SourceModelUUID = remoteApp.SourceModel().Id()
			continue
		}
		// Try looking up the info for the local application.
		_, err = api.st.Application(ep.ApplicationName)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		} else if err == nil {
			result.ApplicationName = ep.ApplicationName
			result.Endpoint = params.RemoteEndpoint{
				Name:      ep.Name,
				Interface: ep.Interface,
				Role:      ep.Role,
				Scope:     ep.Scope,
				Limit:     ep.Limit,
			}
			continue
		}
	}
	return result, nil
}

// Relations returns information about the cross-model relations with the specified keys
// in the local model.
func (api *RemoteRelationsAPI) Relations(entities params.Entities) (params.RemoteRelationResults, error) {
	results := params.RemoteRelationResults{
		Results: make([]params.RemoteRelationResult, len(entities.Entities)),
	}
	for i, entity := range entities.Entities {
		remoteRelation, err := api.remoteRelation(entity)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = remoteRelation
	}
	return results, nil
}

// RemoteApplications returns the current state of the remote applications with
// the specified names in the local model.
func (api *RemoteRelationsAPI) RemoteApplications(entities params.Entities) (params.RemoteApplicationResults, error) {
	results := params.RemoteApplicationResults{
		Results: make([]params.RemoteApplicationResult, len(entities.Entities)),
	}
	one := func(entity params.Entity) (*params.RemoteApplication, error) {
		tag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		remoteApp, err := api.st.RemoteApplication(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		status, err := remoteApp.Status()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &params.RemoteApplication{
			Name:       remoteApp.Name(),
			OfferName:  remoteApp.OfferName(),
			Life:       params.Life(remoteApp.Life().String()),
			Status:     status.Status.String(),
			ModelUUID:  remoteApp.SourceModel().Id(),
			Registered: remoteApp.Registered(),
		}, nil
	}
	for i, entity := range entities.Entities {
		remoteApplication, err := one(entity)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = remoteApplication
	}
	return results, nil
}

// PublishLocalRelationChange publishes local relations changes to the
// remote side offering those relations.
func (api *RemoteRelationsAPI) PublishLocalRelationChange(
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

func (api *RemoteRelationsAPI) publishRelationChange(change params.RemoteRelationChangeEvent) error {
	logger.Debugf("publish into model %v change: %+v", api.st.ModelUUID(), change)

	relationTag, err := api.getRemoteEntityTag(change.RelationId)
	if err != nil {
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

	// If the remote model has destroyed the relation,
	// do it here also.
	if change.Life != params.Alive {
		if err := rel.Destroy(); err != nil {
			return errors.Trace(err)
		}
	}

	// Look up the application on the remote side of this relation
	// ie from the model which published this change.
	applicationTag, err := api.getRemoteEntityTag(change.ApplicationId)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("application tag for remote id %+v is %v", change.ApplicationId, applicationTag)
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

func (api *RemoteRelationsAPI) getRemoteEntityTag(id params.RemoteEntityId) (names.Tag, error) {
	modelTag := names.NewModelTag(id.ModelUUID)
	return api.st.GetRemoteEntity(modelTag, id.Token)
}

// RegisterRemoteRelations sets up the local model to participate
// in the specified relations. This operation is idempotent.
func (api *RemoteRelationsAPI) RegisterRemoteRelations(
	relations params.RegisterRemoteRelations,
) (params.RemoteEntityIdResults, error) {
	results := params.RemoteEntityIdResults{
		Results: make([]params.RemoteEntityIdResult, len(relations.Relations)),
	}
	for i, relation := range relations.Relations {
		if id, err := api.registerRemoteRelation(relation); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		} else {
			results.Results[i].Result = id
		}
	}
	return results, nil
}

func (api *RemoteRelationsAPI) registerRemoteRelation(relation params.RegisterRemoteRelation) (*params.RemoteEntityId, error) {
	logger.Debugf("register remote relation %+v", relation)
	// TODO(wallyworld) - do this as a transaction so the result is atomic
	// Perform some initial validation - is the local application alive?

	// The name the consuming side knows the application by is not necessarily
	// what it has been deployed as locally.
	localApplicationName := relation.OfferedApplicationName
	appOffer, err := api.st.ListOffers(crossmodel.OfferedApplicationFilter{ApplicationName: relation.OfferedApplicationName})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(appOffer) == 0 {
		// TODO(wallyworld) - we don't yet record the offer, assume the URL contains the name
		// return errors.NotFoundf("offered application %q", relation.OfferedApplicationName)
		appNameParts := strings.Split(relation.OfferedApplicationName, "-")
		if len(appNameParts) > 1 {
			localApplicationName = appNameParts[len(appNameParts)-1]
		}
	} else {
		// TODO(wallyworld) - charm name should be service name
		localApplicationName = appOffer[0].CharmName
	}

	localApp, err := api.st.Application(localApplicationName)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get application for offer %q", relation.OfferedApplicationName)
	}
	if localApp.Life() != state.Alive {
		return nil, errors.NotFoundf("application %v", localApplicationName)
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
		Name:        uniqueRemoteApplicationName,
		SourceModel: names.NewModelTag(relation.ApplicationId.ModelUUID),
		Token:       relation.ApplicationId.Token,
		Endpoints:   []charm.Relation{remoteEndpoint.Relation},
		Registered:  true,
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
	token, err := api.st.ExportLocalEntity(names.NewApplicationTag(localApp.Name()))
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, errors.Annotatef(err, "exporting local application %v", localApp.Name())
	}
	logger.Debugf("local application %v from model %v exported with token %v ", localApp.Name(), api.st.ModelUUID(), token)
	return &params.RemoteEntityId{
		ModelUUID: api.st.ModelUUID(),
		Token:     token,
	}, nil
}

// WatchRemoteApplications starts a strings watcher that notifies of the addition,
// removal, and lifecycle changes of remote applications in the model; and
// returns the watcher ID and initial IDs of remote applications, or an error if
// watching failed.
func (api *RemoteRelationsAPI) WatchRemoteApplications() (params.StringsWatchResult, error) {
	w := api.st.WatchRemoteApplications()
	if changes, ok := <-w.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: api.resources.Register(w),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(w)
}

// WatchLocalRelationUnits starts a RelationUnitsWatcher for watching the local
// relation units involved in each specified relation in the local model,
// and returns the watcher IDs and initial values, or an error if the relation
// units could not be watched.
func (api *RemoteRelationsAPI) WatchLocalRelationUnits(args params.Entities) (params.RelationUnitsWatchResults, error) {
	results := params.RelationUnitsWatchResults{
		make([]params.RelationUnitsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		relationTag, err := names.ParseRelationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		w, err := api.watchLocalRelationUnits(relationTag)
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

func (api *RemoteRelationsAPI) watchLocalRelationUnits(tag names.RelationTag) (state.RelationUnitsWatcher, error) {
	relation, err := api.st.KeyRelation(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, ep := range relation.Endpoints() {
		_, err := api.st.Application(ep.ApplicationName)
		if errors.IsNotFound(err) {
			// Not found, so it's the remote application. Try the next endpoint.
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		w, err := relation.WatchUnits(ep.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return w, nil
	}
	return nil, errors.NotFoundf("local application for %s", names.ReadableString(tag))
}

// WatchRemoteApplicationRelations starts a StringsWatcher for watching the relations of
// each specified application in the local model, and returns the watcher IDs
// and initial values, or an error if the services' relations could not be
// watched.
func (api *RemoteRelationsAPI) WatchRemoteApplicationRelations(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		applicationTag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		appName := applicationTag.Id()
		w, err := api.st.WatchRemoteApplicationRelations(appName)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		changes, ok := <-w.Changes()
		if !ok {
			results.Results[i].Error = common.ServerError(watcher.EnsureErr(w))
			continue
		}
		results.Results[i].StringsWatcherId = api.resources.Register(w)
		results.Results[i].Changes = changes
	}
	return results, nil
}

// WatchRemoteRelations starts a strings watcher that notifies of the addition,
// removal, and lifecycle changes of remote relations in the model; and
// returns the watcher ID and initial IDs of remote relations, or an error if
// watching failed.
func (api *RemoteRelationsAPI) WatchRemoteRelations() (params.StringsWatchResult, error) {
	w := api.st.WatchRemoteRelations()
	if changes, ok := <-w.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: api.resources.Register(w),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(w)
}
