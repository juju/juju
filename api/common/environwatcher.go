// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
)

// EnvironWatcher provides common client-side API functions
// to call into apiserver.common.EnvironWatcher.
type EnvironWatcher struct {
	facade base.FacadeCaller
}

// NewEnvironWatcher creates a EnvironWatcher on the specified facade,
// and uses this name when calling through the caller.
func NewEnvironWatcher(facade base.FacadeCaller) *EnvironWatcher {
	return &EnvironWatcher{facade}
}

// WatchForEnvironConfigChanges return a NotifyWatcher waiting for the
// environment configuration to change.
func (e *EnvironWatcher) WatchForEnvironConfigChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := e.facade.FacadeCall("WatchForEnvironConfigChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(e.facade.RawAPICaller(), result), nil
}

// EnvironConfig returns the current environment configuration.
func (e *EnvironWatcher) EnvironConfig() (*config.Config, error) {
	var result params.EnvironConfigResult
	err := e.facade.FacadeCall("EnvironConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}
