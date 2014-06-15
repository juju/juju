// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
)

// EnvironWatcher provides common client-side API functions
// to call into apiserver.common.EnvironWatcher.
type EnvironWatcher struct {
	facadeName string
	caller     base.Caller
}

// NewEnvironWatcher creates a EnvironWatcher on the specified facade,
// and uses this name when calling through the caller.
func NewEnvironWatcher(facadeName string, caller base.Caller) *EnvironWatcher {
	return &EnvironWatcher{facadeName, caller}
}

func (e *EnvironWatcher) call(method string, params, result interface{}) error {
	return e.caller.Call(
		e.facadeName, e.caller.BestFacadeVersion(e.facadeName), "",
		method, params, result)
}

// WatchForEnvironConfigChanges return a NotifyWatcher waiting for the
// environment configuration to change.
func (e *EnvironWatcher) WatchForEnvironConfigChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := e.call("WatchForEnvironConfigChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(e.caller, result), nil
}

// EnvironConfig returns the current environment configuration.
func (e *EnvironWatcher) EnvironConfig() (*config.Config, error) {
	var result params.EnvironConfigResult
	err := e.call("EnvironConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}
