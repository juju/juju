// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/state"
)

type backend struct {
	*state.StatePool
}

// InsertSSHConnRequest inserts a new ssh connection request into the state.
func (b backend) InsertSSHConnRequest(arg state.SSHConnRequestArg) error {
	model, poolHelper, err := b.StatePool.GetModel(arg.ModelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	defer poolHelper.Release()
	return model.State().InsertSSHConnRequest(arg)
}

// RemoveSSHConnRequest removes a ssh connection request from the state.
func (b backend) RemoveSSHConnRequest(arg state.SSHConnRequestRemoveArg) error {
	systemState, err := b.StatePool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	return systemState.RemoveSSHConnRequest(arg)
}

// ControllerMachine returns the specified controller machine.
func (b backend) ControllerMachine(id string) (*state.Machine, error) {
	systemState, err := b.StatePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return systemState.Machine(id)
}

// SSHHostKeys returns the SSH host keys for a machine.
func (b backend) SSHHostKeys(modelUUID string, machineTag names.MachineTag) (state.SSHHostKeys, error) {
	model, poolHelper, err := b.StatePool.GetModel(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer poolHelper.Release()
	return model.State().GetSSHHostKeys(machineTag)
}
