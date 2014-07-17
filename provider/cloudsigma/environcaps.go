// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/constraints"
)

// SupportedArchitectures returns the image architectures which can
// be hosted by this environment.
func (env *environ) SupportedArchitectures() ([]string, error) {
	return []string{arch.AMD64}, nil
}

var unsupportedConstraints = []string{
	constraints.Container,
	constraints.InstanceType,
	constraints.Tags,
}

// ConstraintsValidator returns a Validator instance which
// is used to validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, err
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)
	return validator, nil
}

// SupportNetworks returns whether the environment has support to
// specify networks for services and machines.
func (env *environ) SupportNetworks() bool {
	logger.Debugf("environ:SupportedNetworks")
	return false
}

// SupportsUnitAssignment returns an error which, if non-nil, indicates
// that the environment does not support unit placement. If the environment
// does not support unit placement, then machines may not be created
// without units, and units cannot be placed explcitly.
func (env *environ) SupportsUnitPlacement() error {
	logger.Debugf("cloudsigma:environ:SupportsUnitPlacement not implemented")
	return nil
}
