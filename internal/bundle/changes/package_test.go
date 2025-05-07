// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"testing"

	"github.com/juju/tc"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
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
