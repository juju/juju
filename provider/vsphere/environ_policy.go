// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/utils/arch"

	"github.com/juju/juju/constraints"
)

// PrecheckInstance is part of the environs.Environ interface.
func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement == "" {
		return nil
	}
	return env.withSession(func(env *sessionEnviron) error {
		return env.PrecheckInstance(series, cons, placement)
	})
}

// PrecheckInstance is part of the environs.Environ interface.
func (env *sessionEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	_, err := env.parsePlacement(placement)
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
