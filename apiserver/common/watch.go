// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// AgentEntityWatcher implements a common Watch method for use by
// various facades.
type AgentEntityWatcher struct {
	st          state.EntityFinder
	resources   facade.Resources
	getCanWatch GetAuthFunc
}

// NewAgentEntityWatcher returns a new AgentEntityWatcher. The
// GetAuthFunc will be used on each invocation of Watch to determine
// current permissions.
func NewAgentEntityWatcher(st state.EntityFinder, resources facade.Resources, getCanWatch GetAuthFunc) *AgentEntityWatcher {
	return &AgentEntityWatcher{
		st:          st,
		resources:   resources,
		getCanWatch: getCanWatch,
	}
}

func (a *AgentEntityWatcher) watchEntity(tag names.Tag) (string, error) {
	entity0, err := a.st.FindEntity(tag)
	if err != nil {
		return "", err
	}
	entity, ok := entity0.(state.NotifyWatcherFactory)
	if !ok {
		return "", apiservererrors.NotSupportedError(tag, "watching")
	}
	watch := entity.Watch()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		return a.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

// Watch starts an NotifyWatcher for each given entity.
func (a *AgentEntityWatcher) Watch(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := a.getCanWatch()
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
			watcherId, err = a.watchEntity(tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
