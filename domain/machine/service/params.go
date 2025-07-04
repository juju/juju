// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
)

// CreateMachineArgs contains arguments for creating a machine.
type CreateMachineArgs struct {
	Constraints constraints.Constraints
	Directive   deployment.Placement
	Platform    deployment.Platform
	Nonce       *string
}
