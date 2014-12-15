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
	return nil, errors.Trace(errNotImplemented)
}

// InstanceAvailabilityZoneNames returns the names of the availability
// zones for the specified instances. The error returned follows the same
// rules as Environ.Instances.
func (env *environ) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	return nil, errors.Trace(errNotImplemented)
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
