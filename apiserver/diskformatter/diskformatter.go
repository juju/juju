// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
)

func init() {
	common.RegisterStandardFacade("DiskFormatter", 1, NewDiskFormatterAPI)
}

var logger = loggo.GetLogger("juju.apiserver.diskformatter")

// DiskFormatterAPI provides access to the DiskFormatter API facade.
type DiskFormatterAPI struct {
	st          stateInterface
	resources   *common.Resources
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

var getState = func(st *state.State) stateInterface {
	return stateShim{st}
}

// NewDiskFormatterAPI creates a new client-side DiskFormatter API facade.
func NewDiskFormatterAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*DiskFormatterAPI, error) {

	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}

	getAuthFunc := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}

	return &DiskFormatterAPI{
		st:          getState(st),
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}

// WatchAttachedBlockDevices returns a NotifyWatcher for observing changes
// to each unit's attached block devices.
func (a *DiskFormatterAPI) WatchAttachedBlockDevices(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		unit, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		watcherId := ""
		if canAccess(unit) {
			watcherId, err = a.watchOneAttachedBlockDevices(unit)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (a *DiskFormatterAPI) watchOneAttachedBlockDevices(tag names.UnitTag) (string, error) {
	w, err := a.st.WatchAttachedBlockDevices(tag.Id())
	if err != nil {
		return "", err
	}
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-w.Changes(); ok {
		return a.resources.Register(w), nil
	}
	return "", watcher.EnsureErr(w)
}

// AttachedBlockDevices returns details about each specified unit's attached
// block devices.
func (a *DiskFormatterAPI) AttachedBlockDevices(args params.Entities) (params.BlockDevicesResults, error) {
	result := params.BlockDevicesResults{
		Results: make([]params.BlockDevicesResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.BlockDevicesResults{}, err
	}
	for i, entity := range args.Entities {
		unit, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		var blockDevices []storage.BlockDevice
		if canAccess(unit) {
			blockDevices, err = a.st.AttachedBlockDevices(unit.Id())
		}
		result.Results[i].Result = blockDevices
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (a *DiskFormatterAPI) DiskDatastores(args params.Entities) (params.DatastoreResults, error) {
	result := params.BlockDevicesResults{
		Results: make([]params.BlockDevicesResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.BlockDevicesResults{}, err
	}
	for i, entity := range args.Entities {
		diskTag, err := names.ParseDiskTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		diskTag.Id()

		err = common.ErrPerm
		var blockDevices []storage.BlockDevice
		if canAccess(unit) {
			blockDevices, err = a.st.AttachedBlockDevices(unit.Id())
		}
		result.Results[i].Result = blockDevices
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
