// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ModelWatcher implements two common methods for use by various
// facades - WatchForModelConfigChanges and ModelConfig.
type ModelWatcher struct {
	st         state.ModelAccessor
	resources  *Resources
	authorizer Authorizer
}

// NewModelWatcher returns a new ModelWatcher. Active watchers
// will be stored in the provided Resources. The two GetAuthFunc
// callbacks will be used on each invocation of the methods to
// determine current permissions.
// Right now, environment tags are not used, so both created AuthFuncs
// are called with "" for tag, which means "the current environment".
func NewModelWatcher(st state.ModelAccessor, resources *Resources, authorizer Authorizer) *ModelWatcher {
	return &ModelWatcher{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}
}

// WatchForModelConfigChanges returns a NotifyWatcher that observes
// changes to the environment configuration.
// Note that although the NotifyWatchResult contains an Error field,
// it's not used because we are only returning a single watcher,
// so we use the regular error return.
func (e *ModelWatcher) WatchForModelConfigChanges() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watch := e.st.WatchForModelConfigChanges()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = e.resources.Register(watch)
	} else {
		return result, watcher.EnsureErr(watch)
	}
	return result, nil
}

// ModelConfig returns the current environment's configuration.
func (e *ModelWatcher) ModelConfig() (params.ModelConfigResult, error) {
	result := params.ModelConfigResult{}

	config, err := e.st.ModelConfig()
	if err != nil {
		return result, err
	}
	allAttrs := config.AllAttrs()

	if !e.authorizer.AuthModelManager() {
		// Mask out any secrets in the environment configuration
		// with values of the same type, so it'll pass validation.
		//
		// TODO(dimitern) 201309-26 bug #1231384
		// Delete the code below and mark the bug as fixed,
		// once it's live tested on MAAS and 1.16 compatibility
		// is dropped.
		provider, err := environs.Provider(config.Type())
		if err != nil {
			return result, err
		}
		secretAttrs, err := provider.SecretAttrs(config)
		for k := range secretAttrs {
			allAttrs[k] = "not available"
		}
	}
	result.Config = allAttrs
	return result, nil
}
