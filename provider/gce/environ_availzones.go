// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"path"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/storage"
)

type gceAvailabilityZone struct {
	*compute.Zone
}

func (z *gceAvailabilityZone) Name() string {
	return z.Zone.Name
}

func (z *gceAvailabilityZone) Available() bool {
	return z.Zone.Status == google.StatusUp
}

// AvailabilityZones returns all availability zones in the environment.
func (env *environ) AvailabilityZones(ctx context.ProviderCallContext) (network.AvailabilityZones, error) {
	zones, err := env.gce.AvailabilityZones(ctx, env.cloud.Region)
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}

	var result network.AvailabilityZones
	for _, zone := range zones {
		if zone.Deprecated != nil {
			continue
		}
		result = append(result, &gceAvailabilityZone{Zone: zone})
	}
	return result, nil
}

// InstanceAvailabilityZoneNames returns the names of the availability
// zones for the specified instances. The error returned follows the same
// rules as Environ.Instances.
func (env *environ) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
	instances, err := env.Instances(ctx, ids)
	if err != nil && err != environs.ErrPartialInstances && err != environs.ErrNoInstances {
		return nil, errors.Trace(err)
	}
	// We let the two environs errors pass on through. However, we do
	// not use errors.Trace in that case since callers may not call
	// errors.Cause.

	results := make(map[instance.Id]string, len(ids))
	for _, inst := range instances {
		if eInst, ok := inst.(*environInstance); ok && eInst != nil {
			results[inst.Id()] = path.Base(eInst.base.Zone)
		}
	}

	return results, nil
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	zone, err := env.deriveAvailabilityZones(ctx, args.Placement, args.VolumeAttachments)
	if zone != "" {
		return []string{zone}, errors.Trace(err)
	}
	return nil, errors.Trace(err)
}

func (env *environ) availZone(ctx context.ProviderCallContext, name string) (*compute.Zone, error) {
	zones, err := env.gce.AvailabilityZones(ctx, env.cloud.Region)
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}
	for _, z := range zones {
		if z.Name == name {
			return z, nil
		}
	}
	return nil, errors.NotFoundf("invalid availability zone %q", name)
}

func (env *environ) availZoneUp(ctx context.ProviderCallContext, name string) (*compute.Zone, error) {
	zone, err := env.availZone(ctx, name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if zone.Status != google.StatusUp {
		return nil, errors.Errorf("availability zone %q is %s", zone.Name, zone.Status)
	}
	return zone, nil
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

func (env *environ) instancePlacementZone(ctx context.ProviderCallContext, placement string, volumeAttachmentsZone string) (string, error) {
	if placement == "" {
		return volumeAttachmentsZone, nil
	}
	// placement will always be a zone name or empty.
	zone, err := env.parsePlacement(ctx, placement)
	if err != nil {
		return "", errors.Trace(err)
	}
	if volumeAttachmentsZone != "" && zone.Name != volumeAttachmentsZone {
		return "", errors.Errorf(
			"cannot create instance with placement %q, as this will prevent attaching the requested disks in zone %q",
			placement, volumeAttachmentsZone,
		)
	}
	return zone.Name, nil
}

func (e *environ) deriveAvailabilityZones(
	ctx context.ProviderCallContext,
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
	zone, err := e.parsePlacement(ctx, placement)
	if err != nil {
		return "", err
	}
	if err := validateAvailabilityZoneConsistency(zone.Name, volumeAttachmentsZone); err != nil {
		return "", errors.Annotatef(err, "cannot create instance with placement %q", placement)
	}
	return zone.Name, nil
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
