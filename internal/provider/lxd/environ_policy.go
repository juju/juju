// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
)

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(ctx context.Context, args environs.PrecheckInstanceParams) error {
	_, err := env.deriveAvailabilityZone(ctx,
		environs.StartInstanceParams{
			Placement: args.Placement,
		},
	)
	return errors.Trace(err)
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.Tags,
	constraints.Container,
	constraints.AllocatePublicIP,
	constraints.ImageID,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	validator := constraints.NewValidator()

	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.VirtType, []string{"", "container", "virtual-machine"})

	// Only consume supported juju architectures for this release. This will
	// also remove any duplicate architectures.
	lxdArches := set.NewStrings(env.server().SupportedArches()...)
	supported := set.NewStrings(arch.AllSupportedArches...).Intersection(lxdArches)

	validator.RegisterVocabulary(constraints.Arch, supported.SortedValues())

	return validator, nil
}

// ShouldApplyControllerConstraints returns if bootstrapping logic should use
// default constraints
func (env *environ) ShouldApplyControllerConstraints(cons constraints.Value) bool {
	if cons.HasVirtType() && *cons.VirtType == "virtual-machine" {
		return true
	}
	return false
}

// SupportNetworks returns whether the environment has support to
// specify networks for applications and machines.
func (env *environ) SupportNetworks(ctx context.Context) bool {
	return false
}
