// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if _, err := env.parsePlacement(args.Placement); err != nil {
		return errors.Trace(err)
	}
	return nil
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.Tags,
	constraints.VirtType,
	constraints.Container,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()

	validator.RegisterUnsupported(unsupportedConstraints)

	// TODO(natefinch): This is only correct so long as the lxd is running on
	// the local machine.  If/when we support a remote lxd environment, we'll
	// need to change this to match the arch of the remote machine.
	validator.RegisterVocabulary(constraints.Arch, []string{arch.HostArch()})

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
