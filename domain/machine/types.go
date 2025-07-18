// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
)

const ManualInstancePrefix = "manual:"

// ExportMachine represents a machine that is being exported to another
// controller.
type ExportMachine struct {
	Name         machine.Name
	UUID         machine.UUID
	Nonce        string
	PasswordHash string
	Placement    string
	Base         string
	InstanceID   string
}

// CreateMachineArgs contains arguments for creating a machine.
type CreateMachineArgs struct {
	MachineUUID machine.UUID
	Constraints constraints.Constraints
	Directive   deployment.Placement
	Platform    deployment.Platform
	Nonce       *string
}

// AddMachineArgs contains arguments for adding a machine.
type AddMachineArgs struct {
	Constraints constraints.Constraints
	Directive   deployment.Placement
	Platform    deployment.Platform
	Nonce       *string
}
