// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
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
		return nil, apiservererrors.RestoreError(err)
	}
	return apiwatcher.NewNotifyWatcher(f.caller.RawAPICaller(), result), nil
}

// FanConfig returns the current fan configuration.
func (f *Facade) FanConfig(tag names.MachineTag) (network.FanConfig, error) {
	var (
		result params.FanConfigResult
		err    error
	)
	if f.caller.BestAPIVersion() < 2 {
		err = f.caller.FacadeCall("FanConfig", nil, &result)
	} else {
		arg := params.Entity{Tag: tag.String()}
		err = f.caller.FacadeCall("FanConfig", arg, &result)
	}
	if err != nil {
		return nil, apiservererrors.RestoreError(err)
	}
	return networkingcommon.FanConfigResultToFanConfig(result)
}
