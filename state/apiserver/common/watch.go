// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
)

// AgentEntityWatcher implements a common Watch method for use by
// various facades.
type AgentEntityWatcher struct {
	st          AgentEntityWatcherer
	resources   *Resources
	getCanWatch GetAuthFunc
}

type AgentEntityWatcherer interface {
	// state.State implements AgentEntityWatcher to provide ways for
	// us to call object.Watch (for machines, units, etc). This is
	// used to allow us to test with mocks without having to actually
	// bring up state.
	AgentEntityWatcher(tag string) (state.AgentEntityWatcher, error)
}

// NewAgentEntityWatcher returns a new AgentEntityWatcher. The
// GetAuthFunc will be used on each invocation of Watch to determine
// current permissions.
func NewAgentEntityWatcher(st AgentEntityWatcherer, resources *Resources, getCanWatch GetAuthFunc) *AgentEntityWatcher {
	return &AgentEntityWatcher{
		st:          st,
		resources:   resources,
		getCanWatch: getCanWatch,
	}
}

func (a *AgentEntityWatcher) watchEntity(tag string) (string, error) {
	agentEntityWatcher, err := a.st.AgentEntityWatcher(tag)
	if err != nil {
		return "", err
	}
	watch := agentEntityWatcher.Watch()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		return a.resources.Register(watch), nil
	}
	return "", watcher.MustErr(watch)
}

// Watch starts an NotifyWatcher for each given entity.
func (a *AgentEntityWatcher) Watch(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := a.getCanWatch()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		err := ErrPerm
		watcherId := ""
		if canWatch(entity.Tag) {
			watcherId, err = a.watchEntity(entity.Tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}
