// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"strings"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

type gceAvailabilityZone struct {
	zone *compute.Zone
}

func (z *gceAvailabilityZone) Name() string {
	return z.zone.Name
}

func (z *gceAvailabilityZone) Available() bool {
	// https://cloud.google.com/compute/docs/reference/latest/zones#status
	return z.zone.Status == statusUp
}

// AvailabilityZones returns all availability zones in the environment.
func (env *environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	zones, err := env.gce.availabilityZones(env.ecfg.region())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []common.AvailabilityZone
	for _, zone := range zones {
		result = append(result, &gceAvailabilityZone{zone})
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
	// We let the two environs errors pass on through. However, we do
	// not use errors.Trace in that case since callers may not call
	// errors.Cause.

	results := make([]string, len(ids))
	for i, inst := range instances {
		if eInst := inst.(*environInstance); eInst != nil {
			results[i] = eInst.zone
		}
	}

	return results, err
}

func (env *environ) parseAvailabilityZones(args environs.StartInstanceParams) ([]string, error) {
	if args.Placement != "" {
		// args.Placement will always be a zone name or empty.
		gceZone, err := env.parsePlacement(args.Placement)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !gceZone.Available() {
			return nil, errors.Errorf("availability zone %q is %s", gceZone.Name(), gceZone.zone.Status)
		}
		return []string{gceZone.Name()}, nil
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
	zoneInstances, err := availabilityZoneAllocations(env, group)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("found %d zones: %v", len(zoneInstances), zoneInstances)

	var zoneNames []string
	region := env.ecfg.region()
	for _, z := range zoneInstances {
		if region == "" || strings.HasPrefix(z.ZoneName, region+"-") {
			zoneNames = append(zoneNames, z.ZoneName)
		}
	}

	if len(zoneNames) == 0 {
		return nil, errors.New("failed to determine availability zones")
	}

	return zoneNames, nil
}
