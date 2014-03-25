// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
)

// EnvironWatcher implements two common methods for use by various
// facades - WatchForEnvironConfigChanges and EnvironConfig.
type EnvironWatcher struct {
	st                state.EnvironAccessor
	resources         *Resources
	getCanWatch       GetAuthFunc
	getCanReadSecrets GetAuthFunc
}

// NewEnvironWatcher returns a new EnvironWatcher. Active watchers
// will be stored in the provided Resources. The two GetAuthFunc
// callbacks will be used on each invocation of the methods to
// determine current permissions.
// Right now, environment tags are not used, so both created AuthFuncs
// are called with "" for tag, which means "the current environment".
func NewEnvironWatcher(st state.EnvironAccessor, resources *Resources, getCanWatch, getCanReadSecrets GetAuthFunc) *EnvironWatcher {
	return &EnvironWatcher{
		st:                st,
		resources:         resources,
		getCanWatch:       getCanWatch,
		getCanReadSecrets: getCanReadSecrets,
	}
}

// WatchForEnvironConfigChanges returns a NotifyWatcher that observes
// changes to the environment configuration.
// Note that although the NotifyWatchResult contains an Error field,
// it's not used because we are only returning a single watcher,
// so we use the regular error return.
func (e *EnvironWatcher) WatchForEnvironConfigChanges() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}

	canWatch, err := e.getCanWatch()
	if err != nil {
		return result, err
	}
	// TODO(dimitern) If we have multiple environments in state, use a
	// tag argument here and as a method argument.
	if !canWatch("") {
		return result, ErrPerm
	}

	watch := e.st.WatchForEnvironConfigChanges()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = e.resources.Register(watch)
	} else {
		return result, watcher.MustErr(watch)
	}
	return result, nil
}

// EnvironConfig returns the current environment's configuration.
func (e *EnvironWatcher) EnvironConfig() (params.EnvironConfigResult, error) {
	result := params.EnvironConfigResult{}

	canReadSecrets, err := e.getCanReadSecrets()
	if err != nil {
		return result, err
	}

	config, err := e.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	allAttrs := config.AllAttrs()

	// TODO(dimitern) If we have multiple environments in state, use a
	// tag argument here and as a method argument.
	if !canReadSecrets("") {
		// Mask out any secrets in the environment configuration
		// with values of the same type, so it'll pass validation.
		//
		// TODO(dimitern) 201309-26 bug #1231384
		// Delete the code below and mark the bug as fixed,
		// once it's live tested on MAAS and 1.16 compatibility
		// is dropped.
		env, err := environs.New(config)
		if err != nil {
			return result, err
		}
		secretAttrs, err := env.Provider().SecretAttrs(config)
		for k := range secretAttrs {
			allAttrs[k] = "not available"
		}
	}
	result.Config = allAttrs
	return result, nil
}
