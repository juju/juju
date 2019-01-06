// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/utils/arch"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// PrecheckInstance is part of the environs.Environ interface.
func (env *environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if args.Placement == "" {
		return nil
	}
	return env.withSession(ctx, func(env *sessionEnviron) error {
		return env.PrecheckInstance(ctx, args)
	})
}

// PrecheckInstance is part of the environs.Environ interface.
func (env *sessionEnviron) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	_, err := env.parsePlacement(ctx, args.Placement)
	return err
}

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.VirtType,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, []string{
		arch.AMD64, arch.I386,
	})
	return validator, nil
}
