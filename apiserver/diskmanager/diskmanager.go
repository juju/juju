// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

func init() {
	common.RegisterStandardFacade("DiskManager", 1, NewDiskManagerAPI)
}

var logger = loggo.GetLogger("juju.apiserver.diskmanager")

// DiskManagerAPI provides access to the DiskManager API facade.
type DiskManagerAPI struct {
	st          stateInterface
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

var getState = func(st *state.State) stateInterface {
	return stateShim{st}
}

// NewDiskManagerAPI creates a new server-side DiskManager API facade.
func NewDiskManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*DiskManagerAPI, error) {

	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}

	authEntityTag := authorizer.GetAuthTag()
	getAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			// A machine agent can always access its own machine.
			return tag == authEntityTag
		}, nil
	}

	return &DiskManagerAPI{
		st:          getState(st),
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}

func (d *DiskManagerAPI) SetMachineBlockDevices(args params.SetMachineBlockDevices) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineBlockDevices)),
	}
	canAccess, err := d.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, arg := range args.MachineBlockDevices {
		tag, err := names.ParseMachineTag(arg.Machine)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			err = common.ErrPerm
		} else {
			// TODO(axw) create volumes for block devices without matching
			// volumes, if and only if the block device has a serial. Under
			// the assumption of unique (to a machine) serial IDs, this
			// gives us a guaranteed *persistently* unique way of identifying
			// the volume.
			//
			// NOTE: we must predicate the above on there being no unprovisioned
			// volume attachments for the machine, otherwise we would have
			// a race between the volume attachment info being recorded and
			// the diskmanager publishing block devices and erroneously creating
			// volumes.
			err = d.st.SetMachineBlockDevices(tag.Id(), stateBlockDeviceInfo(arg.BlockDevices))
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func stateBlockDeviceInfo(devices []storage.BlockDevice) []state.BlockDeviceInfo {
	result := make([]state.BlockDeviceInfo, len(devices))
	for i, dev := range devices {
		result[i] = state.BlockDeviceInfo{
			dev.DeviceName,
			dev.Label,
			dev.UUID,
			dev.Serial,
			dev.Size,
			dev.FilesystemType,
			dev.InUse,
			dev.MountPoint,
		}
	}
	return result
}
