// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

type vmwareAvailZone struct {
	r mo.ComputeResource
}

// Name implements common.AvailabilityZone
func (z *vmwareAvailZone) Name() string {
	return z.r.Name
}

// Available implements common.AvailabilityZone
func (z *vmwareAvailZone) Available() bool {
	return true
}

// AvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) AvailabilityZones() (zones []common.AvailabilityZone, err error) {
	err = env.withSession(func(env *sessionEnviron) error {
		zones, err = env.AvailabilityZones()
		return err
	})
	return zones, err
}

// AvailabilityZones is part of the common.ZonedEnviron interface.
func (env *sessionEnviron) AvailabilityZones() ([]common.AvailabilityZone, error) {
	if env.zones == nil {
		computeResources, err := env.client.ComputeResources(env.ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		zones := make([]common.AvailabilityZone, len(computeResources))
		for i, cr := range computeResources {
			zones[i] = &vmwareAvailZone{*cr}
		}
		env.zones = zones
	}
	return env.zones, nil
}

// InstanceAvailabilityZoneNames is part of the common.ZonedEnviron interface.
func (env *environ) InstanceAvailabilityZoneNames(ids []instance.Id) (names []string, err error) {
	err = env.withSession(func(env *sessionEnviron) error {
		names, err = env.InstanceAvailabilityZoneNames(ids)
		return err
	})
	return names, err
}

// InstanceAvailabilityZoneNames is part of the common.ZonedEnviron interface.
func (env *sessionEnviron) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	zones, err := env.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}
	instances, err := env.Instances(ids)
	switch err {
	case nil, environs.ErrPartialInstances:
		break
	case environs.ErrNoInstances:
		return nil, err
	default:
		return nil, errors.Trace(err)
	}

	results := make([]string, len(ids))
	for i, inst := range instances {
		if inst == nil {
			continue
		}
		vm := inst.(*environInstance).base
		for _, zone := range zones {
			cr := &zone.(*vmwareAvailZone).r
			if cr.ResourcePool.Value == vm.ResourcePool.Value {
				results[i] = cr.Name
				break
			}
		}
	}
	return results, err
}

// DeriveAvailabilityZone is part of the common.ZonedEnviron interface.
func (env *environ) DeriveAvailabilityZone(args environs.StartInstanceParams) (names string, err error) {
	err = env.withSession(func(env *sessionEnviron) error {
		names, err = env.DeriveAvailabilityZone(args)
		return err
	})
	return names, err
}

// TODO (HML) 16-oct-2017
// Verify any volume attachments
//
// DeriveAvailabilityZone is part of the common.ZonedEnviron interface.
func (env *sessionEnviron) DeriveAvailabilityZone(args environs.StartInstanceParams) (string, error) {
	zone, err := env.parseAvailabilityZone(args)
	if err != nil {
		return "", err
	}
	return zone, nil
}

func (env *sessionEnviron) availZone(name string) (common.AvailabilityZone, error) {
	zones, err := env.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, z := range zones {
		if z.Name() == name {
			return z, nil
		}
	}
	return nil, errors.NotFoundf("availability zone %q", name)
}

// parseAvailabilityZone returns the availability zone that should be
// tried for the given instance spec. If a placement argument was
// provided then only that one is returned.
func (env *sessionEnviron) parseAvailabilityZone(args environs.StartInstanceParams) (string, error) {
	if args.Placement != "" {
		// args.Placement will always be a zone name or empty.
		placement, err := env.parsePlacement(args.Placement)
		if err != nil {
			return "", errors.Trace(err)
		}
		return placement.Name(), nil
	}
	return args.AvailabilityZone, nil
}
