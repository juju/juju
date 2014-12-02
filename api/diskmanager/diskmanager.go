// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
)

const diskManagerFacade = "DiskManager"

// State provides access to a diskmanager worker's view of the state.
type State struct {
	facade base.FacadeCaller
	tag    names.MachineTag
}

// NewState creates a new client-side DiskManager facade.
func NewState(caller base.APICaller, authTag names.MachineTag) *State {
	return &State{
		base.NewFacadeCaller(caller, diskManagerFacade),
		authTag,
	}
}

// SetMachineBlockDevices sets the block devices attached to the machine
// identified by the authenticated machine tag.
func (st *State) SetMachineBlockDevices(devices []storage.BlockDevice) error {
	args := params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine:      st.tag.String(),
			BlockDevices: devices,
		}},
	}
	var results params.ErrorResults
	err := st.facade.FacadeCall("SetMachineBlockDevices", args, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}
