// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ModelWatcher implements two common methods for use by various
// facades - WatchForModelConfigChanges and ModelConfig.
type ModelWatcher struct {
	st         state.ModelAccessor
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewModelWatcher returns a new ModelWatcher. Active watchers
// will be stored in the provided Resources. The two GetAuthFunc
// callbacks will be used on each invocation of the methods to
// determine current permissions.
// Right now, model tags are not used, so both created AuthFuncs
// are called with "" for tag, which means "the current model".
func NewModelWatcher(st state.ModelAccessor, resources facade.Resources, authorizer facade.Authorizer) *ModelWatcher {
	return &ModelWatcher{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}
}

// WatchForModelConfigChanges returns a NotifyWatcher that observes
// changes to the model configuration.
// Note that although the NotifyWatchResult contains an Error field,
// it's not used because we are only returning a single watcher,
// so we use the regular error return.
func (m *ModelWatcher) WatchForModelConfigChanges(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watch := m.st.WatchForModelConfigChanges()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = m.resources.Register(watch)
	} else {
		return result, watcher.EnsureErr(watch)
	}
	return result, nil
}

// ModelConfig returns the current model's configuration.
func (m *ModelWatcher) ModelConfig(ctx context.Context) (params.ModelConfigResult, error) {
	result := params.ModelConfigResult{}
	config, err := m.st.ModelConfig(ctx)
	if err != nil {
		return result, err
	}
	result.Config = config.AllAttrs()
	return result, nil
}
