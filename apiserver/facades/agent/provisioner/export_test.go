// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import "github.com/juju/juju/apiserver/params"

func NewPrepareOrGetContext(result params.MachineNetworkConfigResults, maintain bool) *prepareOrGetContext {
	return &prepareOrGetContext{result: result, maintain: maintain}
}

func NewContainerProfileContext(result params.ContainerProfileResults, modelName string) *containerProfileContext {
	return &containerProfileContext{result: result, modelName: modelName}
}

func MachineChangeProfileChangeInfo(machine ProfileMachine, st ProfileBackend, unitName string) (params.ProfileChangeResult, error) {
	return machineChangeProfileChangeInfo(machine, st, unitName)
}
