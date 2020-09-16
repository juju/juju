// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

type Facade struct {
	*common.LifeGetter
	*common.AgentEntityWatcher
	resources facade.Resources
	state     CAASFirewallerState
	*common.ApplicationWatcherFacade
}

// NewStateFacadeLegacy provides the signature required for facade registration.
func NewStateFacadeLegacy(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	appWatcherFacade := common.NewApplicationWatcherFacadeFromState(ctx.State(), resources, common.ApplicationFilterCAASLegacy)
	return newFacadeLegacy(
		resources,
		authorizer,
		&stateShim{ctx.State()},
		appWatcherFacade,
	)
}

func newFacadeLegacy(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASFirewallerState,
	applicationWatcherFacade *common.ApplicationWatcherFacade,
) (*Facade, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	accessApplication := common.AuthFuncForTagKind(names.ApplicationTagKind)
	return &Facade{
		LifeGetter: common.NewLifeGetter(
			st, common.AuthAny(
				common.AuthFuncForTagKind(names.ApplicationTagKind),
				common.AuthFuncForTagKind(names.UnitTagKind),
			),
		),
		AgentEntityWatcher: common.NewAgentEntityWatcher(
			st,
			resources,
			accessApplication,
		),
		resources:                resources,
		state:                    st,
		ApplicationWatcherFacade: applicationWatcherFacade,
	}, nil
}

// IsExposed returns whether the specified applications are exposed.
func (f *Facade) IsExposed(args params.Entities) (params.BoolResults, error) {
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
func (f *Facade) ApplicationsConfig(args params.Entities) (params.ApplicationGetConfigResults, error) {
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

// FacadeEmbedded provides access to the CAASFireWaller API facade for embedded applications.
type FacadeEmbedded struct {
	*Facade

	accessModel common.GetAuthFunc
}

// NewStateFacadeEmbedded provides the signature required for facade registration.
func NewStateFacadeEmbedded(ctx facade.Context) (*FacadeEmbedded, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	appWatcherFacade := common.NewApplicationWatcherFacadeFromState(ctx.State(), resources, common.ApplicationFilterCAASEmbedded)
	return newFacadeEmbedded(
		resources,
		authorizer,
		&stateShim{ctx.State()},
		appWatcherFacade,
	)
}

func newFacadeEmbedded(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASFirewallerState,
	applicationWatcherFacade *common.ApplicationWatcherFacade,
) (*FacadeEmbedded, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	accessApplication := common.AuthFuncForTagKind(names.ApplicationTagKind)

	return &FacadeEmbedded{
		accessModel: common.AuthFuncForTagKind(names.ModelTagKind),
		Facade: &Facade{
			LifeGetter: common.NewLifeGetter(
				st, common.AuthAny(
					common.AuthFuncForTagKind(names.ApplicationTagKind),
					common.AuthFuncForTagKind(names.UnitTagKind),
				),
			),
			AgentEntityWatcher: common.NewAgentEntityWatcher(
				st,
				resources,
				accessApplication,
			),
			resources:                resources,
			state:                    st,
			ApplicationWatcherFacade: applicationWatcherFacade,
		},
	}, nil
}

// WatchOpenedPorts returns a new StringsWatcher for each given
// model tag.
func (f *FacadeEmbedded) WatchOpenedPorts(args params.Entities) (params.StringsWatchResults, error) {
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

func (f *FacadeEmbedded) watchOneModelOpenedPorts(tag names.Tag) (string, []string, error) {
	// NOTE: tag is ignored, as there is only one model in the
	// state DB. Once this changes, change the code below accordingly.
	watch := f.state.WatchOpenedPorts()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return f.resources.Register(watch), changes, nil
	}
	return "", nil, watcher.EnsureErr(watch)
}
