// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// EnvironWatcher provides common client side api functions
// to call into the apiserver.common.EnvironWatcher.
type EnvironWatcher struct {
	façadeName string
	caller     base.Caller
}

// NewEnvironWatcher creates a EnvironWatcher on the specified façade,
// and uses this name when calling through the caller.
func NewEnvironWatcher(façadeName string, caller base.Caller) *EnvironWatcher {
	return &EnvironWatcher{façadeName, caller}
}

// WatchForEnvironConfigChanges return a NotifyWatcher waiting for the
// environment configuration to change.
func (e *EnvironWatcher) WatchForEnvironConfigChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := e.caller.Call(e.façadeName, "", "WatchForEnvironConfigChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(e.caller, result)
	return w, nil
}

// EnvironConfig returns the current environment configuration.
func (e *EnvironWatcher) EnvironConfig() (*config.Config, error) {
	var result params.EnvironConfigResult
	err := e.caller.Call(e.façadeName, "", "EnvironConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}
