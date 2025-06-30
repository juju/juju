// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/deployment"
)

const ManualInstancePrefix = "manual:"

// ExportMachine represents a machine that is being exported to another
// controller.
type ExportMachine struct {
	Name  machine.Name
	UUID  machine.UUID
	Nonce string
}

// CreateMachineArgs contains arguments for creating a machine.
type CreateMachineArgs struct {
	MachineUUID machine.UUID
	Platform    deployment.Platform
	Nonce       *string
}

// PlaceMachineArgs contains arguments for placing a machine.
type PlaceMachineArgs struct {
	Directive deployment.Placement
	Platform  deployment.Platform
	Nonce     *string
}
