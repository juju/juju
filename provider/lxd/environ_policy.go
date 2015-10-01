// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/network"
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
	localArch := arch.HostArch()
	return []string{localArch}, nil
}

var unsupportedConstraints = []string{
	constraints.CpuCores,
	constraints.CpuPower,
	//constraints.Mem,
	constraints.InstanceType,
	constraints.Tags,
}

// conflictingConstraints declares the contraints that will be verified.
var conflictingConstraints = []string{
	constraints.Arch,      // Arches
	constraints.Container, // VirtType
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()

	// conflicts

	validator.RegisterConflicts(
		nil,
		//[]string{constraints.InstanceType},
		conflictingConstraints,
	)

	// unsupported

	validator.RegisterUnsupported(unsupportedConstraints)

	// vocab

	supportedArches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, errors.Trace(err)
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)

	// TODO(ericsnow) Register valid container types (e.g. lxd)?

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
