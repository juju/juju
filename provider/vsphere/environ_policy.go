// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// PrecheckInstance is part of the environs.Environ interface.
func (env *environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if args.Placement == "" && args.Constraints.String() == "" {
		return nil
	}
	return env.withSession(ctx, func(env *sessionEnviron) error {
		return env.PrecheckInstance(ctx, args)
	})
}

// PrecheckInstance is part of the environs.Environ interface.
func (env *sessionEnviron) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if _, err := env.parsePlacement(ctx, args.Placement); err != nil {
		return errors.Trace(err)
	}
	if err := env.checkZones(ctx, args.Constraints.Zones); err != nil {
		return errors.Trace(err)
	}
	if err := env.checkDatastore(ctx, args.Constraints.RootDiskSource); err != nil {
		return errors.Trace(err)
	}
	err := env.checkExtendPermissions(ctx, args.Constraints.RootDisk)
	return errors.Trace(err)
}

func (env *sessionEnviron) checkZones(ctx context.ProviderCallContext, zones *[]string) error {
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

func (env *sessionEnviron) checkDatastore(ctx context.ProviderCallContext, datastore *string) error {
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

func (env *sessionEnviron) checkExtendPermissions(ctx context.ProviderCallContext, rootDisk *uint64) error {
	if rootDisk == nil || *rootDisk == 0 {
		return nil
	}
	// If we're going to need to resize the root disk we need to have
	// the System.Read privilege on the root level folder - this seems
	// to be because the extend disk task doesn't get created with a
	// target, so the root-level permissions are applied.
	ok, err := env.client.UserHasRootLevelPrivilege(env.ctx, "System.Read")
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		user := env.cloud.Credential.Attributes()[credAttrUser]
		return errors.Errorf("the System.Read privilege is required at the root level to extend disks - please grant the ReadOnly role to %q", user)
	}
	return nil
}

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.VirtType,
}

// ConstraintsValidator returns a Validator value which is used to
// validate and merge constraints.
func (env *environ) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterVocabulary(constraints.Arch, []string{
		arch.AMD64, arch.I386,
	})
	return validator, nil
}
