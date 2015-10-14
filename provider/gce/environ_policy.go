// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if _, err := env.parsePlacement(placement); err != nil {
		return errors.Trace(err)
	}

	if cons.HasInstanceType() {
		if !checkInstanceType(cons) {
			return errors.Errorf("invalid GCE instance type %q", *cons.InstanceType)
		}
	}

	return nil
}

// SupportedArchitectures returns the image architectures which can
// be hosted by this environment.
func (env *environ) SupportedArchitectures() ([]string, error) {
	env.archLock.Lock()
	defer env.archLock.Unlock()

	if env.supportedArchitectures != nil {
		return env.supportedArchitectures, nil
	}

	archList, err := env.lookupArchitectures()
	if err != nil {
		return nil, errors.Trace(err)
	}
	env.supportedArchitectures = archList
	return archList, nil
}

var supportedArchitectures = common.SupportedArchitectures

func (env *environ) lookupArchitectures() ([]string, error) {
	// Create a filter to get all images from our region and for the
	// correct stream.
	cloudSpec, err := env.Region()
	if err != nil {
		return nil, errors.Trace(err)
	}
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
		Stream:    env.Config().ImageStream(),
	})
	archList, err := supportedArchitectures(env, imageConstraint)
	return archList, errors.Trace(err)
}

var unsupportedConstraints = []string{
	constraints.Tags,
	// TODO(dimitern: Replace Networks with Spaces in a follow-up.
	constraints.Networks,
}

// instanceTypeConstraints defines the fields defined on each of the
// instance types.  See instancetypes.go.
var instanceTypeConstraints = []string{
	constraints.Arch, // Arches
	constraints.CpuCores,
	constraints.CpuPower,
	constraints.Mem,
	constraints.Container, // VirtType
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()

	// conflicts

	// TODO(ericsnow) Are these correct?
	validator.RegisterConflicts(
		[]string{constraints.InstanceType},
		instanceTypeConstraints,
	)

	// unsupported

	validator.RegisterUnsupported(unsupportedConstraints)

	// vocab

	supportedArches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, errors.Trace(err)
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)

	instTypeNames := make([]string, len(allInstanceTypes))
	for i, itype := range allInstanceTypes {
		instTypeNames[i] = itype.Name
	}
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)

	validator.RegisterVocabulary(constraints.Container, []string{vtype})

	return validator, nil
}

// environ provides SupportsUnitPlacement (a method of the
// state.EnvironCapatability interface) by embedding
// common.SupportsUnitPlacementPolicy.

// SupportNetworks returns whether the environment has support to
// specify networks for services and machines.
func (env *environ) SupportNetworks() bool {
	return false
}

// SupportAddressAllocation takes a network.Id and returns a bool
// and an error. The bool indicates whether that network supports
// static ip address allocation.
func (env *environ) SupportAddressAllocation(netID network.Id) (bool, error) {
	return false, nil
}
