// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/utils/arch"

	"github.com/juju/juju/constraints"
)

var unsupportedConstraints = []string{
	constraints.Container,
	constraints.InstanceType,
	constraints.Tags,
	constraints.VirtType,
}

// This is provided to avoid double hard-coding
// of provider specific architecture for
// use in constraints validator and metadata lookup params
// (used to validate images).
var providerSupportedArchitectures = []string{arch.AMD64}

// ConstraintsValidator returns a Validator instance which
// is used to validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, providerSupportedArchitectures)
	return validator, nil
}

// SupportNetworks returns whether the environment has support to
// specify networks for applications and machines.
func (env *environ) SupportNetworks() bool {
	return false
}
