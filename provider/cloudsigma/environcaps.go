// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/provider/common"
)

func (env *environ) SupportedArchitectures() ([]string, error) {
	env.archMutex.Lock()
	defer env.archMutex.Unlock()
	if env.supportedArchitectures != nil {
		return env.supportedArchitectures, nil
	}
	logger.Debugf("Getting supported architectures from simplestream.")
	cloudSpec, err := env.Region()
	if err != nil {
		return nil, err
	}
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
		Stream:    env.Config().ImageStream(),
	})
	env.supportedArchitectures, err = common.SupportedArchitectures(env, imageConstraint)
	logger.Debugf("Supported architectures: %v", env.supportedArchitectures)
	return env.supportedArchitectures, err
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
	return false
}

// SupportsUnitAssignment returns an error which, if non-nil, indicates
// that the environment does not support unit placement. If the environment
// does not support unit placement, then machines may not be created
// without units, and units cannot be placed explcitly.
func (env *environ) SupportsUnitPlacement() error {
	return errors.NotImplementedf("SupportsUnitPlacement")
}
