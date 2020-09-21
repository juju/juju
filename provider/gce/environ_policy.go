// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	volumeAttachmentsZone, err := volumeAttachmentsZone(args.VolumeAttachments)
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := env.instancePlacementZone(ctx, args.Placement, volumeAttachmentsZone); err != nil {
		return errors.Trace(err)
	}

	if args.Constraints.HasInstanceType() {
		if !checkInstanceType(args.Constraints) {
			return errors.Errorf("invalid GCE instance type %q", *args.Constraints.InstanceType)
		}
	}

	return nil
}

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.VirtType,
	constraints.AllocatePublicIP,
}

// instanceTypeConstraints defines the fields defined on each of the
// instance types.  See instancetypes.go.
var instanceTypeConstraints = []string{
	constraints.Arch, // Arches
	constraints.Cores,
	constraints.CpuPower,
	constraints.Mem,
	constraints.Container, // VirtType
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
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
func (env *environ) SupportNetworks(ctx context.ProviderCallContext) bool {
	return false
}
