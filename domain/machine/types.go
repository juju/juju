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
	// Platform is the platform to use for the machine.
	Platform deployment.Platform

	// Nonce is the provided nonce for the machine.
	Nonce *string
}
