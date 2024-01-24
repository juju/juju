// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/domain/blockdevice"
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
func (st *State) SetMachineBlockDevices(devices []blockdevice.BlockDevice) error {
	args := params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine:      st.tag.String(),
			BlockDevices: devices,
		}},
	}
	var results params.ErrorResults
	err := st.facade.FacadeCall(context.TODO(), "SetMachineBlockDevices", args, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}
