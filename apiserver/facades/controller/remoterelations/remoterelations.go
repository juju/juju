// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state/watcher"
)

// API provides access to version 1 of the remote relations API facade.
type APIv1 struct {
	*API
}

// API provides access to the remote relations API facade.
type API struct {
	*common.ControllerConfigAPI
	st         RemoteRelationsState
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewAPI creates a new server-side API facade backed by global state.
func NewAPIv1(ctx facade.Context) (*APIv1, error) {
	api, err := NewAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{api}, nil
}

// NewAPI creates a new server-side API facade backed by global state.
func NewAPI(ctx facade.Context) (*API, error) {
	return NewRemoteRelationsAPI(
		stateShim{st: ctx.State(), Backend: commoncrossmodel.GetBackend(ctx.State())},
		common.NewStateControllerConfig(ctx.State()),
		ctx.Resources(), ctx.Auth(),
	)
}

// NewRemoteRelationsAPI returns a new server-side API facade.
func NewRemoteRelationsAPI(
	st RemoteRelationsState,
	controllerCfgAPI *common.ControllerConfigAPI,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &API{
		st:                  st,
		ControllerConfigAPI: controllerCfgAPI,
		resources:           resources,
		authorizer:          authorizer,
	}, nil
}

// ImportRemoteEntities adds entities to the remote entities collection with the specified opaque tokens.
func (api *API) ImportRemoteEntities(args params.RemoteEntityTokenArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := api.importRemoteEntity(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (api *API) importRemoteEntity(arg params.RemoteEntityTokenArg) error {
	entityTag, err := names.ParseTag(arg.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	return api.st.ImportRemoteEntity(entityTag, arg.Token)
}

// ExportEntities allocates unique, remote entity IDs for the given entities in the local model.
func (api *API) ExportEntities(entities params.Entities) (params.TokenResults, error) {
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
func (api *API) GetTokens(args params.GetTokenArgs) (params.StringResults, error) {
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
func (api *API) SaveMacaroons(args params.EntityMacaroonArgs) (params.ErrorResults, error) {
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

// RelationUnitSettings returns the relation unit settings for the
// given relation units in the local model. (Removed in v2 of the API
// - the settings are included in the events from
// WatchLocalRelationChanges.)
func (api *APIv1) RelationUnitSettings(relationUnits params.RelationUnits) (params.SettingsResults, error) {
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
				return nil, errors.Errorf("invalid relation setting %q: expected string, got %T", k, v)
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

func (api *API) remoteRelation(entity params.Entity) (*params.RemoteRelation, error) {
	tag, err := names.ParseRelationTag(entity.Tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rel, err := api.st.KeyRelation(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &params.RemoteRelation{
		Id:        rel.Id(),
		Life:      life.Value(rel.Life().String()),
		Suspended: rel.Suspended(),
		Key:       tag.Id(),
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
func (api *API) Relations(entities params.Entities) (params.RemoteRelationResults, error) {
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
func (api *API) RemoteApplications(entities params.Entities) (params.RemoteApplicationResults, error) {
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
		mac, err := remoteApp.Macaroon()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &params.RemoteApplication{
			Name:            remoteApp.Name(),
			OfferUUID:       remoteApp.OfferUUID(),
			Life:            life.Value(remoteApp.Life().String()),
			ModelUUID:       remoteApp.SourceModel().Id(),
			IsConsumerProxy: remoteApp.IsConsumerProxy(),
			Macaroon:        mac,
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
func (api *API) WatchRemoteApplications() (params.StringsWatchResult, error) {
	w := api.st.WatchRemoteApplications()
	// TODO(jam): 2019-10-27 Watching Changes() should be protected with a select with api.ctx.Cancel()
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
// units could not be watched. WatchLocalRelationUnits is only supported on the v1 API - later versions provide WatchLocalRelationChanges instead.
func (api *APIv1) WatchLocalRelationUnits(args params.Entities) (params.RelationUnitsWatchResults, error) {
	results := params.RelationUnitsWatchResults{
		Results: make([]params.RelationUnitsWatchResult, len(args.Entities)),
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
		// TODO(jam): 2019-10-27 Watching Changes() should be protected with a select with api.ctx.Cancel()
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

// WatchLocalRelationChanges starts a RemoteRelationWatcher for each
// specified relation, returning the watcher IDs and initial values,
// or an error if the remote relations couldn't be watched.
func (api *API) WatchLocalRelationChanges(args params.Entities) (params.RemoteRelationWatchResults, error) {
	results := params.RemoteRelationWatchResults{
		make([]params.RemoteRelationWatchResult, len(args.Entities)),
	}

	watchOne := func(arg params.Entity) (common.RelationUnitsWatcher, params.RemoteRelationChangeEvent, error) {
		var empty params.RemoteRelationChangeEvent
		relationTag, err := names.ParseRelationTag(arg.Tag)
		if err != nil {
			return nil, empty, errors.Trace(err)
		}
		relationToken, appToken, err := commoncrossmodel.GetConsumingRelationTokens(api.st, relationTag)
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

	for i, arg := range args.Entities {
		w, changes, err := watchOne(arg)
		if err != nil {
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
// WatchLocalRelationChanges doesn't exist before the v2 API.
func (api *APIv1) WatchLocalRelationChanges(_, _ struct{}) {}

// WatchRemoteApplicationRelations starts a StringsWatcher for watching the relations of
// each specified application in the local model, and returns the watcher IDs
// and initial values, or an error if the services' relations could not be
// watched.
func (api *API) WatchRemoteApplicationRelations(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
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
		// TODO(jam): 2019-10-27 Watching Changes() should be protected with a select with api.ctx.Cancel()
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
func (api *API) WatchRemoteRelations() (params.StringsWatchResult, error) {
	w := api.st.WatchRemoteRelations()
	// TODO(jam): 2019-10-27 Watching Changes() should be protected with a select with api.ctx.Cancel()
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
func (api *API) ConsumeRemoteRelationChanges(changes params.RemoteRelationsChanges) (params.ErrorResults, error) {
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

// SetRemoteApplicationsStatus sets the status for the specified remote applications.
func (api *API) SetRemoteApplicationsStatus(args params.SetStatus) (params.ErrorResults, error) {
	var result params.ErrorResults
	result.Results = make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		remoteAppTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		app, err := api.st.RemoteApplication(remoteAppTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		statusValue := status.Status(entity.Status)
		if statusValue == status.Terminated {
			operation := app.TerminateOperation(entity.Info)
			err = api.st.ApplyOperation(operation)
		} else {
			err = app.SetStatus(status.StatusInfo{
				Status:  statusValue,
				Message: entity.Info,
			})
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// UpdateControllersForModels is not available via the V1 API.
func (u *APIv1) UpdateControllersForModels(_, _ struct{}) {}

// UpdateControllersForModels changes the external controller records for the
// associated model entities. This is used when the remote relations worker gets
// redirected following migration of an offering model.
func (api *API) UpdateControllersForModels(args params.UpdateControllersForModelsParams) (params.ErrorResults, error) {
	var result params.ErrorResults
	result.Results = make([]params.ErrorResult, len(args.Changes))

	for i, change := range args.Changes {
		cInfo := change.Info

		modelTag, err := names.ParseModelTag(change.ModelTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		controllerTag, err := names.ParseControllerTag(cInfo.ControllerTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		controller := crossmodel.ControllerInfo{
			ControllerTag: controllerTag,
			Alias:         cInfo.Alias,
			Addrs:         cInfo.Addrs,
			CACert:        cInfo.CACert,
		}

		if err := api.st.UpdateControllerForModel(controller, modelTag.Id()); err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}

	return result, nil
}
