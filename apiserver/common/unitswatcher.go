// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// UnitsWatcher implements a common WatchUnits method for use by
// various facades.
type UnitsWatcher struct {
	st          state.EntityFinder
	resources   facade.Resources
	getCanWatch GetAuthFunc
}

// NewUnitsWatcher returns a new UnitsWatcher. The GetAuthFunc will be
// used on each invocation of WatchUnits to determine current
// permissions.
func NewUnitsWatcher(st state.EntityFinder, resources facade.Resources, getCanWatch GetAuthFunc) *UnitsWatcher {
	return &UnitsWatcher{
		st:          st,
		resources:   resources,
		getCanWatch: getCanWatch,
	}
}

func (u *UnitsWatcher) watchOneEntityUnits(canWatch AuthFunc, tag names.Tag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	if !canWatch(tag) {
		return nothing, apiservererrors.ErrPerm
	}
	entity0, err := u.st.FindEntity(tag)
	if err != nil {
		return nothing, err
	}
	entity, ok := entity0.(state.UnitsWatcher)
	if !ok {
		return nothing, apiservererrors.NotSupportedError(tag, "watching units")
	}
	watch := entity.WatchUnits()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: u.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// WatchUnits starts a StringsWatcher to watch all units belonging to
// to any entity (machine or service) passed in args.
func (u *UnitsWatcher) WatchUnits(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := u.getCanWatch(ctx)
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		entityResult, err := u.watchOneEntityUnits(canWatch, tag)
		result.Results[i] = entityResult
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
