// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/watcher"
)

// Facade provides access to the FanConfigurer API facade.
type Facade struct {
	caller base.FacadeCaller
}

// NewFacade creates a new client-side FanConfigu	er facade.
func NewFacade(caller base.APICaller) *Facade {
	return &Facade{
		caller: base.NewFacadeCaller(caller, "FanConfigurer"),
	}
}

// WatchForFanConfigChanges return a NotifyWatcher waiting for the
// fan configuration to change.
func (f *Facade) WatchForFanConfigChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := f.caller.FacadeCall("WatchForFanConfigChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	return apiwatcher.NewNotifyWatcher(f.caller.RawAPICaller(), result), nil
}

// FanConfig returns the current fan configuration.
func (f *Facade) FanConfig() (network.FanConfig, error) {
	var result params.FanConfigResult
	err := f.caller.FacadeCall("FanConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	return networkingcommon.FanConfigResultToFanConfig(result)
}
