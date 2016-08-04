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

func (env *environ) SupportedArchitectures(knownArchitectures []string) ([]string, error) {
	logger.Debugf("Getting supported architectures from simplestream.")
	cloudSpec, err := env.Region()
	if err != nil {
		return nil, err
	}
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
		Stream:    env.Config().ImageStream(),
	})
	supportedArchitectures, err := common.SupportedArchitectures(env, imageConstraint, knownArchitectures)
	logger.Debugf("Supported architectures: %v", supportedArchitectures)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return supportedArchitectures, nil
}

var unsupportedConstraints = []string{
	constraints.Container,
	constraints.InstanceType,
	constraints.Tags,
	constraints.VirtType,
}

// ConstraintsValidator returns a Validator instance which
// is used to validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := env.SupportedArchitectures(env.initialArchitectures)
	if err != nil {
		return nil, err
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)
	return validator, nil
}

// SupportNetworks returns whether the environment has support to
// specify networks for applications and machines.
func (env *environ) SupportNetworks() bool {
	return false
}
