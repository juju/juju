// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundlechanges

func NewAddMachineParamsMachine(id string) AddMachineParams {
	return AddMachineParams{
		machineID: id,
	}
}

func NewAddMachineParamsContainer(baseMachine, id string) AddMachineParams {
	return AddMachineParams{
		machineID:          baseMachine,
		containerMachineID: id,
	}
}
