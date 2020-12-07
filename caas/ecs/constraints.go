// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/context"
)

var unsupportedConstraints = []string{
	// TODO: review!!!!!!!
	constraints.Cores,
	constraints.VirtType,
	constraints.Container,
	constraints.Arch,
	constraints.InstanceType,
	constraints.Spaces,
	constraints.AllocatePublicIP,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	return validator, nil
}
