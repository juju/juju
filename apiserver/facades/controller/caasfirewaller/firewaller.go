// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	charmscommon "github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationLife looks up the life of the specified application.
	GetApplicationLife(context.Context, string) (life.Value, error)

	// GetUnitLife looks up the life of the specified unit.
	GetUnitLife(context.Context, unit.Name) (life.Value, error)

	// GetApplicationIDByName returns a application ID by application name. It
	// returns an error if the application can not be found by the name.
	GetApplicationIDByName(ctx context.Context, name string) (application.ID, error)

	// IsApplicationExposed returns whether the provided application is exposed or not.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	IsApplicationExposed(ctx context.Context, appName string) (bool, error)
}

type Facade struct {
	*common.AgentEntityWatcher
	resources       facade.Resources
	watcherRegistry facade.WatcherRegistry
	state           CAASFirewallerState
	charmInfoAPI    *charmscommon.CharmInfoAPI
	appCharmInfoAPI *charmscommon.ApplicationCharmInfoAPI
	accessModel     common.GetAuthFunc
	accessUnit      common.GetAuthFunc

	applicationService ApplicationService
}

// CharmInfo returns information about the requested charm.
func (f *Facade) CharmInfo(ctx context.Context, args params.CharmURL) (params.Charm, error) {
	return f.charmInfoAPI.CharmInfo(ctx, args)
}

// ApplicationCharmInfo returns information about an application's charm.
func (f *Facade) ApplicationCharmInfo(ctx context.Context, args params.Entity) (params.Charm, error) {
	return f.appCharmInfoAPI.ApplicationCharmInfo(ctx, args)
}

// IsExposed returns whether the specified applications are exposed.
func (f *Facade) IsExposed(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	results := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		exposed, err := f.isExposed(ctx, arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = exposed
	}
	return results, nil
}

func (f *Facade) isExposed(ctx context.Context, tagString string) (bool, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return false, errors.Trace(err)
	}
	return f.applicationService.IsApplicationExposed(ctx, tag.Id())
}

// ApplicationsConfig returns the config for the specified applications.
func (f *Facade) ApplicationsConfig(ctx context.Context, args params.Entities) (params.ApplicationGetConfigResults, error) {
	results := params.ApplicationGetConfigResults{
		Results: make([]params.ConfigResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		result, err := f.getApplicationConfig(arg.Tag)
		results.Results[i].Config = result
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (f *Facade) getApplicationConfig(tagString string) (map[string]interface{}, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return nil, errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return app.ApplicationConfig()
}

// WatchApplications starts a StringsWatcher to watch applications
// deployed to this model.
func (f *Facade) WatchApplications(ctx context.Context) (params.StringsWatchResult, error) {
	watch := f.state.WatchApplications()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: f.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}

func NewFacade(
	resources facade.Resources,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	st CAASFirewallerState,
	commonCharmsAPI *charmscommon.CharmInfoAPI,
	appCharmInfoAPI *charmscommon.ApplicationCharmInfoAPI,
	applicationService ApplicationService,
) (*Facade, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	accessApplication := common.AuthFuncForTagKind(names.ApplicationTagKind)
	accessUnit := common.AuthAny(
		common.AuthFuncForTagKind(names.ApplicationTagKind),
		common.AuthFuncForTagKind(names.UnitTagKind),
	)

	return &Facade{
		accessModel: common.AuthFuncForTagKind(names.ModelTagKind),
		accessUnit:  accessUnit,
		AgentEntityWatcher: common.NewAgentEntityWatcher(
			st,
			watcherRegistry,
			accessApplication,
		),
		resources:          resources,
		watcherRegistry:    watcherRegistry,
		state:              st,
		charmInfoAPI:       commonCharmsAPI,
		appCharmInfoAPI:    appCharmInfoAPI,
		applicationService: applicationService,
	}, nil
}

// Life returns the life status of the specified applications or units.
func (f *Facade) Life(ctx context.Context, args params.Entities) (params.LifeResults, error) {
	result := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := f.accessUnit()
	if err != nil {
		return params.LifeResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canRead(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		var lifeValue life.Value
		switch tag.Kind() {
		case names.ApplicationTagKind:
			lifeValue, err = f.applicationService.GetApplicationLife(ctx, tag.Id())
			if errors.Is(err, applicationerrors.ApplicationNotFound) {
				err = errors.NotFoundf("application %s", tag.Id())
			}
		case names.UnitTagKind:
			var unitName unit.Name
			unitName, err = unit.NewName(tag.Id())
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			lifeValue, err = f.applicationService.GetUnitLife(ctx, unitName)
			if errors.Is(err, applicationerrors.UnitNotFound) {
				err = errors.NotFoundf("unit %s", unitName)
			}
		default:
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		result.Results[i].Life = lifeValue
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
