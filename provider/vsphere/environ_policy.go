// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/utils/arch"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
)

// PrecheckInstance is part of the environs.Environ interface.
func (env *environ) PrecheckInstance(args environs.PrecheckInstanceParams) error {
	if args.Placement == "" {
		return nil
	}
	return env.withSession(func(env *sessionEnviron) error {
		return env.PrecheckInstance(args)
	})
}

// PrecheckInstance is part of the environs.Environ interface.
func (env *sessionEnviron) PrecheckInstance(args environs.PrecheckInstanceParams) error {
	_, err := env.parsePlacement(args.Placement)
	return err
}

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.VirtType,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, []string{
		arch.AMD64, arch.I386,
	})
	return validator, nil
}
