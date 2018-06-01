// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/storage"
)

// AvailabilityZones returns all availability zones in the environment.
func (env *environ) AvailabilityZones(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
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
func (env *environ) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
	instances, err := env.Instances(ctx, ids)
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

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	zone, err := env.deriveAvailabilityZones(args.Placement, args.VolumeAttachments)
	if zone != "" {
		return []string{zone}, errors.Trace(err)
	}
	return nil, errors.Trace(err)
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

func (env *environ) instancePlacementZone(placement string, volumeAttachmentsZone string) (string, error) {
	if placement == "" {
		return volumeAttachmentsZone, nil
	}
	// placement will always be a zone name or empty.
	instPlacement, err := env.parsePlacement(placement)
	if err != nil {
		return "", errors.Trace(err)
	}
	if volumeAttachmentsZone != "" && instPlacement.Zone.Name() != volumeAttachmentsZone {
		return "", errors.Errorf(
			"cannot create instance with placement %q, as this will prevent attaching the requested disks in zone %q",
			placement, volumeAttachmentsZone,
		)
	}
	return instPlacement.Zone.Name(), nil
}

func (e *environ) deriveAvailabilityZones(
	placement string,
	volumeAttachments []storage.VolumeAttachmentParams,
) (string, error) {
	volumeAttachmentsZone, err := volumeAttachmentsZone(volumeAttachments)
	if err != nil {
		return "", errors.Trace(err)
	}
	if placement == "" {
		return volumeAttachmentsZone, nil
	}
	instPlacement, err := e.parsePlacement(placement)
	if err != nil {
		return "", err
	}
	instanceZone := instPlacement.Zone.Name()
	if err := validateAvailabilityZoneConsistency(instanceZone, volumeAttachmentsZone); err != nil {
		return "", errors.Annotatef(err, "cannot create instance with placement %q", placement)
	}
	return instanceZone, nil
}

func validateAvailabilityZoneConsistency(instanceZone, volumeAttachmentsZone string) error {
	if volumeAttachmentsZone != "" && instanceZone != volumeAttachmentsZone {
		return errors.Errorf(
			"cannot create instance in zone %q, as this will prevent attaching the requested disks in zone %q",
			instanceZone, volumeAttachmentsZone,
		)
	}
	return nil
}
