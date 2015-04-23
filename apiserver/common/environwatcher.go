// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// EnvironWatcher implements two common methods for use by various
// facades - WatchForEnvironConfigChanges and EnvironConfig.
type EnvironWatcher struct {
	st         state.EnvironAccessor
	resources  *Resources
	authorizer Authorizer
}

// NewEnvironWatcher returns a new EnvironWatcher. Active watchers
// will be stored in the provided Resources. The two GetAuthFunc
// callbacks will be used on each invocation of the methods to
// determine current permissions.
// Right now, environment tags are not used, so both created AuthFuncs
// are called with "" for tag, which means "the current environment".
func NewEnvironWatcher(st state.EnvironAccessor, resources *Resources, authorizer Authorizer) *EnvironWatcher {
	return &EnvironWatcher{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}
}

// WatchForEnvironConfigChanges returns a NotifyWatcher that observes
// changes to the environment configuration.
// Note that although the NotifyWatchResult contains an Error field,
// it's not used because we are only returning a single watcher,
// so we use the regular error return.
func (e *EnvironWatcher) WatchForEnvironConfigChanges() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watch := e.st.WatchForEnvironConfigChanges()
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

// EnvironConfig returns the current environment's configuration.
func (e *EnvironWatcher) EnvironConfig() (params.EnvironConfigResult, error) {
	result := params.EnvironConfigResult{}

	config, err := e.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	allAttrs := config.AllAttrs()

	if !e.authorizer.AuthEnvironManager() {
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
