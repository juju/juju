// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
)

// PrecheckInstance verifies that the provided series and constraints
// are valid for use in creating an instance in this environment.
func (env *environ) PrecheckInstance(ctx context.Context, args environs.PrecheckInstanceParams) error {
	volumeAttachmentsZone, err := volumeAttachmentsZone(args.VolumeAttachments)
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := env.instancePlacementZone(ctx, args.Placement, volumeAttachmentsZone); err != nil {
		return errors.Trace(err)
	}

	if args.Constraints.HasInstanceType() {
		if !env.checkInstanceType(ctx, args.Constraints) {
			return errors.Errorf("invalid GCE instance type %q", *args.Constraints.InstanceType)
		}
	}

	return nil
}

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.VirtType,
	constraints.ImageID,
}

// instanceTypeConstraints defines the fields defined on each of the
// instance types. See instancetypes.go.
var instanceTypeConstraints = []string{
	// TODO: move to a dynamic conflict for arch when gce supports more than amd64
	//constraints.Arch, // Arches
	constraints.Cores,
	constraints.CpuPower,
	constraints.Mem,
	constraints.Container, // VirtType
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	validator := constraints.NewValidator()

	validator.RegisterConflicts(
		[]string{constraints.InstanceType},
		instanceTypeConstraints,
	)

	validator.RegisterUnsupported(unsupportedConstraints)

	// vocab

	instTypesAndCosts, err := env.InstanceTypes(ctx, constraints.Value{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	instTypeNames := make([]string, len(instTypesAndCosts.InstanceTypes))
	for i, itype := range instTypesAndCosts.InstanceTypes {
		instTypeNames[i] = itype.Name
	}
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)
	validator.RegisterVocabulary(constraints.Container, []string{virtType})

	return validator, nil
}

// SupportNetworks returns whether the environment has support to
// specify networks for applications and machines.
func (env *environ) SupportNetworks(ctx context.Context) bool {
	return false
}
