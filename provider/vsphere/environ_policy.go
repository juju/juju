// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
)

// PrecheckInstance is part of the environs.Environ interface.
func (env *environ) PrecheckInstance(ctx envcontext.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if args.Placement == "" && args.Constraints.String() == "" {
		return nil
	}
	return env.withSession(ctx, func(env *sessionEnviron) error {
		return env.PrecheckInstance(ctx, args)
	})
}

// PrecheckInstance is part of the environs.Environ interface.
func (env *sessionEnviron) PrecheckInstance(ctx envcontext.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if _, err := env.parsePlacement(ctx, args.Placement); err != nil {
		return errors.Trace(err)
	}
	if err := env.checkZones(ctx, args.Constraints.Zones); err != nil {
		return errors.Trace(err)
	}
	if err := env.checkDatastore(ctx, args.Constraints.RootDiskSource); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// checkZones ensures all the zones (in the constraints) are valid
// availability zones.
func (env *sessionEnviron) checkZones(ctx envcontext.ProviderCallContext, zones *[]string) error {
	if zones == nil || len(*zones) == 0 {
		return nil
	}
	foundZones, err := env.AvailabilityZones(ctx)
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

func (env *sessionEnviron) checkDatastore(ctx envcontext.ProviderCallContext, datastore *string) error {
	if datastore == nil || *datastore == "" {
		return nil
	}
	name := *datastore
	datastores, err := env.accessibleDatastores(ctx)
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
func (env *environ) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, []string{
		arch.AMD64,
	})
	return validator, nil
}
