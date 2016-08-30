// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
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

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.VirtType,
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

	instTypeNames := make([]string, len(allInstanceTypes))
	for i, itype := range allInstanceTypes {
		instTypeNames[i] = itype.Name
	}
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)

	validator.RegisterVocabulary(constraints.Container, []string{vtype})

	return validator, nil
}

// SupportNetworks returns whether the environment has support to
// specify networks for applications and machines.
func (env *environ) SupportNetworks() bool {
	return false
}
