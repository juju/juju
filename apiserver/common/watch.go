// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// AgentEntityWatcher implements a common Watch method for use by
// various facades.
type AgentEntityWatcher struct {
	st              state.EntityFinder
	watcherRegistry facade.WatcherRegistry
	getCanWatch     GetAuthFunc
}

// NewAgentEntityWatcher returns a new AgentEntityWatcher. The
// GetAuthFunc will be used on each invocation of Watch to determine
// current permissions.
//
// Deprecated: Create the methods directly on the facade instead, for the
// entities that need to be watched!
func NewAgentEntityWatcher(st state.EntityFinder, watcherRegistry facade.WatcherRegistry, getCanWatch GetAuthFunc) *AgentEntityWatcher {
	return &AgentEntityWatcher{
		st:              st,
		watcherRegistry: watcherRegistry,
		getCanWatch:     getCanWatch,
	}
}

// Watch starts an NotifyWatcher for each given entity.
func (a *AgentEntityWatcher) Watch(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := a.getCanWatch(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		watcherId := ""
		if canWatch(tag) {
			watcherId, err = a.watchEntity(ctx, tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (a *AgentEntityWatcher) watchEntity(ctx context.Context, tag names.Tag) (string, error) {
	switch tag.Kind() {
	case names.UnitTagKind, names.MachineTagKind:
		return "", errors.NotSupportedf("watching %s", tag.Kind())
	default:
		return a.legacyWatchEntity(ctx, tag)
	}
}

func (a *AgentEntityWatcher) legacyWatchEntity(ctx context.Context, tag names.Tag) (string, error) {
	entity0, err := a.st.FindEntity(tag)
	if err != nil {
		return "", err
	}
	entity, ok := entity0.(state.NotifyWatcherFactory)
	if !ok {
		return "", apiservererrors.NotSupportedError(tag, "watching")
	}
	watch := entity.Watch()
	id, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, a.watcherRegistry, watch)
	return id, err
}
