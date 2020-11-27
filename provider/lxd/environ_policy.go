// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	_, err := env.parsePlacement(ctx, args.Placement)
	return errors.Trace(err)
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.Tags,
	constraints.VirtType,
	constraints.Container,
	constraints.AllocatePublicIP,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()

	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, env.server().SupportedArches())

	return validator, nil
}

// ShouldApplyControllerConstraints returns if bootstrapping logic should use
// default constraints
func (env *environ) ShouldApplyControllerConstraints() bool {
	return false
}

// SupportNetworks returns whether the environment has support to
// specify networks for applications and machines.
func (env *environ) SupportNetworks(ctx context.ProviderCallContext) bool {
	return false
}
