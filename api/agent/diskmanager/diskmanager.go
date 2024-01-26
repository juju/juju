// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const diskManagerFacade = "DiskManager"

// State provides access to a diskmanager worker's view of the state.
type State struct {
	facade base.FacadeCaller
	tag    names.MachineTag
}

// NewState creates a new client-side DiskManager facade.
func NewState(caller base.APICaller, authTag names.MachineTag, options ...Option) *State {
	return &State{
		base.NewFacadeCaller(caller, diskManagerFacade, options...),
		authTag,
	}
}

// SetMachineBlockDevices sets the block devices attached to the machine
// identified by the authenticated machine tag.
// XXX
func (st *State) SetMachineBlockDevices(devices []blockdevice.BlockDevice) error {
	args := params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine:      st.tag.String(),
			BlockDevices: blockDevicesToParams(devices),
		}},
	}
	var results params.ErrorResults
	err := st.facade.FacadeCall(context.TODO(), "SetMachineBlockDevices", args, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}

func blockDevicesToParams(in []blockdevice.BlockDevice) []params.BlockDevice {
	if len(in) == 0 {
		return nil
	}
	out := make([]params.BlockDevice, len(in))
	for i, d := range in {
		out[i] = params.BlockDevice{
			DeviceName:     d.DeviceName,
			DeviceLinks:    d.DeviceLinks,
			Label:          d.Label,
			UUID:           d.UUID,
			HardwareId:     d.HardwareId,
			WWN:            d.WWN,
			BusAddress:     d.BusAddress,
			Size:           d.SizeMiB,
			FilesystemType: d.FilesystemType,
			InUse:          d.InUse,
			MountPoint:     d.MountPoint,
			SerialId:       d.SerialId,
		}
	}
	return out
}
