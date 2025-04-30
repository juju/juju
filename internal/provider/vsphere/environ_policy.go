// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
)

// PrecheckInstance is part of the environs.Environ interface.
func (env *environ) PrecheckInstance(ctx context.Context, args environs.PrecheckInstanceParams) error {
	if args.Placement == "" && args.Constraints.String() == "" {
		return nil
	}
	return env.withSession(ctx, func(senv *sessionEnviron) error {
		return senv.PrecheckInstance(ctx, args)
	})
}

// PrecheckInstance is part of the environs.Environ interface.
func (senv *sessionEnviron) PrecheckInstance(ctx context.Context, args environs.PrecheckInstanceParams) error {
	if _, err := senv.parsePlacement(ctx, args.Placement); err != nil {
		return errors.Trace(err)
	}
	if err := senv.checkZones(ctx, args.Constraints.Zones); err != nil {
		return errors.Trace(err)
	}
	if err := senv.checkDatastore(ctx, args.Constraints.RootDiskSource); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// checkZones ensures all the zones (in the constraints) are valid
// availability zones.
func (senv *sessionEnviron) checkZones(ctx context.Context, zones *[]string) error {
	if zones == nil || len(*zones) == 0 {
		return nil
	}
	foundZones, err := senv.AvailabilityZones(ctx)
	if err != nil {
		return errors.Trace(err)
	}
constraintZones:
	for _, zone := range *zones {
		for _, foundZone := range foundZones {
			if zone == foundZone.Name() {
				continue constraintZones
			}
		}
		return errors.NotFoundf("availability zone %q", zone)
	}
	return nil
}

func (senv *sessionEnviron) checkDatastore(ctx context.Context, datastore *string) error {
	if datastore == nil || *datastore == "" {
		return nil
	}
	name := *datastore
	datastores, err := senv.accessibleDatastores(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, ds := range datastores {
		if name == ds.Name {
			return nil
		}
	}
	return errors.NotFoundf("datastore %q", name)
}

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.VirtType,
	constraints.AllocatePublicIP,
	constraints.ImageID,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, []string{
		arch.AMD64,
	})
	return validator, nil
}
