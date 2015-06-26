// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/govmomi/vim25/mo"

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

// AvailabilityZones returns all availability zones in the environment.
func (env *environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	zones, err := env.client.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []common.AvailabilityZone
	for _, zone := range zones {
		result = append(result, &vmwareAvailZone{*zone})
	}
	return result, nil
}

// InstanceAvailabilityZoneNames returns the names of the availability
// zones for the specified instances. The error returned follows the same
// rules as Environ.Instances.
func (env *environ) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	instances, err := env.Instances(ids)
	if err != nil && err != environs.ErrPartialInstances && err != environs.ErrNoInstances {
		return nil, errors.Trace(err)
	}

	zones, err := env.client.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]string, 0, len(ids))
	for _, inst := range instances {
		for _, zone := range zones {
			if eInst := inst.(*environInstance); eInst != nil && zone.ResourcePool.Value == eInst.base.ResourcePool.Value {
				results = append(results, zone.Name)
				break
			}
		}
	}

	return results, err
}

func (env *environ) availZone(name string) (*vmwareAvailZone, error) {
	zones, err := env.client.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, z := range zones {
		if z.Name == name {
			return &vmwareAvailZone{*z}, nil
		}
	}
	return nil, errors.NotFoundf("invalid availability zone %q", name)
}

//this variable is exported, because it has to be rewritten in external unit tests
var AvailabilityZoneAllocations = common.AvailabilityZoneAllocations

// parseAvailabilityZones returns the availability zones that should be
// tried for the given instance spec. If a placement argument was
// provided then only that one is returned. Otherwise the environment is
// queried for available zones. In that case, the resulting list is
// roughly ordered such that the environment's instances are spread
// evenly across the region.
func (env *environ) parseAvailabilityZones(args environs.StartInstanceParams) ([]string, error) {
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
	zoneInstances, err := AvailabilityZoneAllocations(env, group)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("found %d zones: %v", len(zoneInstances), zoneInstances)

	var zoneNames []string
	for _, z := range zoneInstances {
		zoneNames = append(zoneNames, z.ZoneName)
	}

	if len(zoneNames) == 0 {
		return nil, errors.NotFoundf("failed to determine availability zones")
	}

	return zoneNames, nil
}
