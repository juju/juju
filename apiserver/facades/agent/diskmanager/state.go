// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import "github.com/juju/juju/state"

type stateInterface interface {
	SetMachineBlockDevices(machineId string, devices []state.BlockDeviceInfo) error
}

type stateShim struct {
	*state.State
}

func (s stateShim) SetMachineBlockDevices(machineId string, devices []state.BlockDeviceInfo) error {
	m, err := s.State.Machine(machineId)
	if err != nil {
		return err
	}
	return m.SetMachineBlockDevices(devices...)
}
