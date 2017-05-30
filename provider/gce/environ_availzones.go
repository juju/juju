// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/storage"
)

// AvailabilityZones returns all availability zones in the environment.
func (env *environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	zones, err := env.gce.AvailabilityZones(env.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []common.AvailabilityZone
	for _, zone := range zones {
		if zone.Deprecated() {
			continue
		}
		// We make a copy since the loop var keeps the same pointer.
		zoneCopy := zone
		result = append(result, &zoneCopy)
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
			results[i] = eInst.base.ZoneName
		}
	}

	return results, err
}

func (env *environ) availZone(name string) (*google.AvailabilityZone, error) {
	zones, err := env.gce.AvailabilityZones(env.cloud.Region)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, z := range zones {
		if z.Name() == name {
			return &z, nil
		}
	}
	return nil, errors.NotFoundf("invalid availability zone %q", name)
}

func (env *environ) availZoneUp(name string) (*google.AvailabilityZone, error) {
	zone, err := env.availZone(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !zone.Available() {
		return nil, errors.Errorf("availability zone %q is %s", zone.Name(), zone.Status())
	}
	return zone, nil
}

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

// startInstanceAvailabilityZones returns the availability zones that
// should be tried for the given instance spec. If a placement argument
// was provided then only that one is returned. Otherwise the environment
// is queried for available zones. In that case, the resulting list is
// roughly ordered such that the environment's instances are spread
// evenly across the region.
func (env *environ) startInstanceAvailabilityZones(args environs.StartInstanceParams) ([]string, error) {
	volumeAttachmentsZone, err := volumeAttachmentsZone(args.VolumeAttachments)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if args.Placement != "" {
		// args.Placement will always be a zone name or empty.
		placement, err := env.parsePlacement(args.Placement)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if volumeAttachmentsZone != "" && placement.Zone.Name() != volumeAttachmentsZone {
			return nil, errors.Errorf(
				"cannot create instance with placement %q, as this will prevent attaching disks in zone %q",
				args.Placement, volumeAttachmentsZone,
			)
		}
		// TODO(ericsnow) Fail if placement.Zone is not in the env's configured region?
		return []string{placement.Zone.Name()}, nil
	}

	if volumeAttachmentsZone != "" {
		return []string{volumeAttachmentsZone}, nil
	}

	// If no availability zone is specified, then automatically spread across
	// the known zones for optimal spread across the instance distribution
	// group.
	var group []instance.Id
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
	for _, z := range zoneInstances {
		zoneNames = append(zoneNames, z.ZoneName)
	}

	if len(zoneNames) == 0 {
		return nil, errors.NotFoundf("failed to determine availability zones")
	}

	return zoneNames, nil
}

// volumeAttachmentsZone determines the availability zone for each volume
// identified in the volume attachment parameters, checking that they are
// all the same, and returns the availability zone name.
func volumeAttachmentsZone(volumeAttachments []storage.VolumeAttachmentParams) (string, error) {
	var zone string
	for i, a := range volumeAttachments {
		volumeZone, _, err := parseVolumeId(a.VolumeId)
		if err != nil {
			return "", errors.Trace(err)
		}
		if zone == "" {
			zone = volumeZone
		} else if zone != volumeZone {
			return "", errors.Errorf(
				"cannot attach volumes from multiple availability zones: %s is in %s, %s is in %s",
				volumeAttachments[i-1].VolumeId, zone, a.VolumeId, volumeZone,
			)
		}
	}
	return zone, nil
}
