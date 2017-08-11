// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

// RemoteRelationsAPI provides access to the RemoteRelations API facade.
type RemoteRelationsAPI struct {
	*common.ControllerConfigAPI
	st         RemoteRelationsState
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewStateRemoteRelationsAPI creates a new server-side RemoteRelationsAPI facade
// backed by global state.
func NewStateRemoteRelationsAPI(ctx facade.Context) (*RemoteRelationsAPI, error) {
	return NewRemoteRelationsAPI(
		stateShim{st: ctx.State(), Backend: commoncrossmodel.GetBackend(ctx.State())},
		common.NewStateControllerConfig(ctx.State()),
		ctx.Resources(), ctx.Auth(),
	)

}

// NewRemoteRelationsAPI returns a new server-side RemoteRelationsAPI facade.
func NewRemoteRelationsAPI(
	st RemoteRelationsState,
	controllerCfgAPI *common.ControllerConfigAPI,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*RemoteRelationsAPI, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &RemoteRelationsAPI{
		st:                  st,
		ControllerConfigAPI: controllerCfgAPI,
		resources:           resources,
		authorizer:          authorizer,
	}, nil
}

// ImportRemoteEntities adds entities to the remote entities collection with the specified opaque tokens.
func (api *RemoteRelationsAPI) ImportRemoteEntities(args params.RemoteEntityTokenArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := api.importRemoteEntity(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (api *RemoteRelationsAPI) importRemoteEntity(arg params.RemoteEntityTokenArg) error {
	entityTag, err := names.ParseTag(arg.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	return api.st.ImportRemoteEntity(entityTag, arg.Token)
}

// ExportEntities allocates unique, remote entity IDs for the given entities in the local model.
func (api *RemoteRelationsAPI) ExportEntities(entities params.Entities) (params.TokenResults, error) {
	results := params.TokenResults{
		Results: make([]params.TokenResult, len(entities.Entities)),
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
		results.Results[i].Token = token
	}
	return results, nil
}

// GetTokens returns the token associated with the entities with the given tags for the given models.
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
		token, err := api.st.GetToken(entityTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
		results.Results[i].Result = token
	}
	return results, nil
}

// SaveMacaroons saves the macaroons for the given entities.
func (api *RemoteRelationsAPI) SaveMacaroons(args params.EntityMacaroonArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		entityTag, err := names.ParseTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		err = api.st.SaveMacaroon(entityTag, arg.Macaroon)
		results.Results[i].Error = common.ServerError(err)
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
			result.RemoteApplicationName = remoteApp.Name()
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
		mac, err := remoteApp.Macaroon()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &params.RemoteApplication{
			Name:       remoteApp.Name(),
			OfferUUID:  remoteApp.OfferUUID(),
			Life:       params.Life(remoteApp.Life().String()),
			Status:     status.Status.String(),
			ModelUUID:  remoteApp.SourceModel().Id(),
			Registered: remoteApp.IsConsumerProxy(),
			Macaroon:   mac,
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
		w, err := commoncrossmodel.WatchRelationUnits(api.st, relationTag)
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

// ConsumeRemoteRelationChanges consumes changes to settings originating
// from the remote/offering side of relations.
func (api *RemoteRelationsAPI) ConsumeRemoteRelationChanges(
	changes params.RemoteRelationsChanges,
) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(changes.Changes)),
	}
	for i, change := range changes.Changes {
		relationTag, err := api.st.GetRemoteEntity(change.RelationToken)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := commoncrossmodel.PublishRelationChange(api.st, relationTag, change); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}
