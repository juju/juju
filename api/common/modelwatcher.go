// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/watcher"
)

// ModelWatcher provides common client-side API functions
// to call into apiserver.common.ModelWatcher.
type ModelWatcher struct {
	facade base.FacadeCaller
}

// NewModelWatcher creates a ModelWatcher on the specified facade,
// and uses this name when calling through the caller.
func NewModelWatcher(facade base.FacadeCaller) *ModelWatcher {
	return &ModelWatcher{facade}
}

// WatchForModelConfigChanges return a NotifyWatcher waiting for the
// model configuration to change.
func (e *ModelWatcher) WatchForModelConfigChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := e.facade.FacadeCall("WatchForModelConfigChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	return apiwatcher.NewNotifyWatcher(e.facade.RawAPICaller(), result), nil
}

// ModelConfig returns the current model configuration.
func (e *ModelWatcher) ModelConfig() (*config.Config, error) {
	var result params.ModelConfigResult
	err := e.facade.FacadeCall("ModelConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}
