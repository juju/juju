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

	if args.Constraints.HasInstanceType() {
		return errors.Errorf("LXD does not support instance types (got %q)", *args.Constraints.InstanceType)
	}

	return nil
}

var unsupportedConstraints = []string{
	constraints.Cores,
	constraints.CpuPower,
	//TODO(ericsnow) Add constraints.Mem as unsupported?
	constraints.InstanceType,
	constraints.Tags,
	constraints.VirtType,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()

	// Register conflicts.

	// We don't have any conflicts to register.

	// Register unsupported constraints.

	validator.RegisterUnsupported(unsupportedConstraints)

	// Register the constraints vocab.

	// TODO(natefinch): This is only correct so long as the lxd is running on
	// the local machine.  If/when we support a remote lxd environment, we'll
	// need to change this to match the arch of the remote machine.
	validator.RegisterVocabulary(constraints.Arch, []string{arch.HostArch()})

	// TODO(ericsnow) Get this working...
	//validator.RegisterVocabulary(constraints.Container, supportedContainerTypes)

	return validator, nil
}

// SupportNetworks returns whether the environment has support to
// specify networks for applications and machines.
func (env *environ) SupportNetworks(ctx context.ProviderCallContext) bool {
	return false
}
