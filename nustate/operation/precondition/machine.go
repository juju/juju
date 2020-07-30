// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package precondition

import (
	"github.com/juju/juju/nustate/persistence/transaction"
)

type MachineAlivePrecondition struct {
	transaction.BuildingBlock

	MachineID string
}

func MachineAlive(machineID string) MachineAlivePrecondition {
	return MachineAlivePrecondition{MachineID: machineID}
}
