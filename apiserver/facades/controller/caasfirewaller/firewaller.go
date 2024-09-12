// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

// ApplicationService provides access to the application service.
type ApplicationService interface {
	GetCharmByApplicationName(context.Context, string) (charm.Charm, applicationcharm.CharmOrigin, application.Platform, error)
	GetApplicationLife(context.Context, string) (life.Value, error)
	GetUnitLife(context.Context, string) (life.Value, error)
}
type Facade struct {
	*common.AgentEntityWatcher
	resources       facade.Resources
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
		exposed, err := f.isExposed(f.state, arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = exposed
	}
	return results, nil
}

func (f *Facade) isExposed(backend CAASFirewallerState, tagString string) (bool, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return false, errors.Trace(err)
	}
	app, err := backend.Application(tag.Id())
	if err != nil {
		return false, errors.Trace(err)
	}
	return app.IsExposed(), nil
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
			resources,
			accessApplication,
		),
		resources:          resources,
		state:              st,
		charmInfoAPI:       commonCharmsAPI,
		appCharmInfoAPI:    appCharmInfoAPI,
		applicationService: applicationService,
	}, nil
}

// Life returns the life status of the specified units.
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
			lifeValue, err = f.applicationService.GetUnitLife(ctx, tag.Id())
			if errors.Is(err, applicationerrors.UnitNotFound) {
				err = errors.NotFoundf("unit %s", tag.Id())
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

// WatchOpenedPorts returns a new StringsWatcher for each given
// model tag.
func (f *Facade) WatchOpenedPorts(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := f.accessModel()
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canWatch(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		watcherID, initial, err := f.watchOneModelOpenedPorts(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].StringsWatcherId = watcherID
		result.Results[i].Changes = initial
	}
	return result, nil
}

func (f *Facade) watchOneModelOpenedPorts(tag names.Tag) (string, []string, error) {
	// NOTE: tag is ignored, as there is only one model in the
	// state DB. Once this changes, change the code below accordingly.
	watch := f.state.WatchOpenedPorts()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return f.resources.Register(watch), changes, nil
	}
	return "", nil, watcher.EnsureErr(watch)
}

// GetOpenedPorts returns all the opened ports for each given application tag.
func (f *Facade) GetOpenedPorts(ctx context.Context, arg params.Entity) (params.ApplicationOpenedPortsResults, error) {
	result := params.ApplicationOpenedPortsResults{
		Results: make([]params.ApplicationOpenedPortsResult, 1),
	}

	appTag, err := names.ParseApplicationTag(arg.Tag)
	if err != nil {
		result.Results[0].Error = apiservererrors.ServerError(err)
		return result, nil
	}

	app, err := f.state.Application(appTag.Id())
	if err != nil {
		result.Results[0].Error = apiservererrors.ServerError(err)
		return result, nil
	}
	openedPortRanges, err := app.OpenedPortRanges()
	if err != nil {
		result.Results[0].Error = apiservererrors.ServerError(err)
		return result, nil
	}
	for endpointName, pgs := range openedPortRanges {
		result.Results[0].ApplicationPortRanges = append(
			result.Results[0].ApplicationPortRanges,
			f.applicationOpenedPortsForEndpoint(endpointName, pgs),
		)
	}
	sort.Slice(result.Results[0].ApplicationPortRanges, func(i, j int) bool {
		// For test.
		return result.Results[0].ApplicationPortRanges[i].Endpoint < result.Results[0].ApplicationPortRanges[j].Endpoint
	})
	return result, nil
}

func (f *Facade) applicationOpenedPortsForEndpoint(endpointName string, pgs []network.PortRange) params.ApplicationOpenedPorts {
	network.SortPortRanges(pgs)
	o := params.ApplicationOpenedPorts{
		Endpoint:   endpointName,
		PortRanges: make([]params.PortRange, len(pgs)),
	}
	for i, pg := range pgs {
		o.PortRanges[i] = params.FromNetworkPortRange(pg)
	}
	return o
}
