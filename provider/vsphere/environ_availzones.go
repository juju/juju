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

// parseAvailabilityZones returns the availability zones that should be
// tried for the given instance spec. If a placement argument was
// provided then only that one is returned. Otherwise the environment is
// queried for available zones. In that case, the resulting list is
// roughly ordered such that the environment's instances are spread
// evenly across the region.
func (env *sessionEnviron) parseAvailabilityZones(args environs.StartInstanceParams) ([]string, error) {
	if args.Placement != "" {
		// args.Placement will always be a zone name or empty.
		placement, err := env.parsePlacement(args.Placement)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []string{placement.Name()}, nil
	}

	// If no availability zone is specified, then automatically spread across
	// the known zones for optimal spread across the instance distribution
	// group.
	var group []instance.Id
	var err error
	if args.DistributionGroup != nil {
		group, err = args.DistributionGroup()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	var zoneNames []string
	// vSphere will misbehave if we call AvailabilityZoneAllocations with empty
	// groups, in this case all zones should be returned.
	if len(group) != 0 {
		zoneInstances, err := common.AvailabilityZoneAllocations(env, group)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, z := range zoneInstances {
			zoneNames = append(zoneNames, z.ZoneName)
		}
	} else {
		zones, err := env.AvailabilityZones()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, z := range zones {
			zoneNames = append(zoneNames, z.Name())
		}
	}
	logger.Infof("found %d zones: %v", len(zoneNames), zoneNames)

	if len(zoneNames) == 0 {
		return nil, errors.NotFoundf("availability zones")
	}

	return zoneNames, nil
}
