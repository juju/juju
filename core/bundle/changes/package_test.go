// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

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
