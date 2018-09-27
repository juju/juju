// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/logfwd/syslog"
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
		return nil, errors.Trace(err)
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conf, nil
}

// WatchForLogForwardConfigChanges return a NotifyWatcher waiting for the
// log forward syslog configuration to change.
func (e *ModelWatcher) WatchForLogForwardConfigChanges() (watcher.NotifyWatcher, error) {
	// TODO(wallyworld) - lp:1602237 - this needs to have it's own backend implementation.
	// For now, we'll piggyback off the ModelConfig API.
	return e.WatchForModelConfigChanges()
}

// LogForwardConfig returns the current log forward syslog configuration.
func (e *ModelWatcher) LogForwardConfig() (*syslog.RawConfig, bool, error) {
	// TODO(wallyworld) - lp:1602237 - this needs to have it's own backend implementation.
	// For now, we'll piggyback off the ModelConfig API.
	modelConfig, err := e.ModelConfig()
	if err != nil {
		return nil, false, err
	}
	cfg, ok := modelConfig.LogFwdSyslog()
	return cfg, ok, nil
}

// UpdateStatusHookInterval returns the current update status hook interval.
func (e *ModelWatcher) UpdateStatusHookInterval() (time.Duration, error) {
	// TODO(wallyworld) - lp:1602237 - this needs to have it's own backend implementation.
	// For now, we'll piggyback off the ModelConfig API.
	modelConfig, err := e.ModelConfig()
	if err != nil {
		return 0, err
	}
	return modelConfig.UpdateStatusHookInterval(), nil
}

// WatchUpdateStatusHookInterval returns a NotifyWatcher that fires when the
// update status hook interval changes.
func (e *ModelWatcher) WatchUpdateStatusHookInterval() (watcher.NotifyWatcher, error) {
	// TODO(wallyworld) - lp:1602237 - this needs to have it's own backend implementation.
	// For now, we'll piggyback off the ModelConfig API.
	return e.WatchForModelConfigChanges()
}
