// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	"path"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/gce/internal/google"
	"github.com/juju/juju/internal/storage"
)

type gceAvailabilityZone struct {
	*computepb.Zone
}

func (z *gceAvailabilityZone) Name() string {
	return z.Zone.GetName()
}

func (z *gceAvailabilityZone) Available() bool {
	return z.Zone.GetStatus() == google.StatusUp
}

// AvailabilityZones returns all availability zones in the environment.
func (env *environ) AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	zones, err := env.gce.AvailabilityZones(ctx, env.cloud.Region)
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}

	var result network.AvailabilityZones
	for _, zone := range zones {
		if zone.GetDeprecated() != nil {
			continue
		}
		result = append(result, &gceAvailabilityZone{Zone: zone})
	}
	return result, nil
}

// InstanceAvailabilityZoneNames returns the names of the availability
// zones for the specified instances. The error returned follows the same
// rules as Environ.Instances.
func (env *environ) InstanceAvailabilityZoneNames(ctx context.Context, ids []instance.Id) (map[instance.Id]string, error) {
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
			results[inst.Id()] = path.Base(eInst.base.GetZone())
		}
	}

	return results, nil
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) DeriveAvailabilityZones(ctx context.Context, args environs.StartInstanceParams) ([]string, error) {
	volumeAttachmentsZone, err := volumeAttachmentsZone(args.VolumeAttachments)
	if err != nil {
		return nil, errors.Trace(err)
	}

	placementZone, err := env.instancePlacementZone(args.Placement, volumeAttachmentsZone)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if placementZone == "" {
		placementZone = args.AvailabilityZone
	}
	if placementZone == "" {
		return nil, nil
	}

	// Validate and check state of the AvailabilityZone
	zone, err := env.availZoneUp(ctx, placementZone)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []string{zone.GetName()}, nil
}

func (env *environ) availZone(ctx context.Context, name string) (*computepb.Zone, error) {
	zones, err := env.gce.AvailabilityZones(ctx, env.cloud.Region)
	if err != nil {
		return nil, env.HandleCredentialError(ctx, err)
	}
	for _, z := range zones {
		if z.GetName() == name {
			return z, nil
		}
	}
	return nil, errors.NotFoundf("invalid availability zone %q", name)
}

func (env *environ) availZoneUp(ctx context.Context, name string) (*computepb.Zone, error) {
	zone, err := env.availZone(ctx, name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if zone.GetStatus() != google.StatusUp {
		return nil, errors.Errorf("availability zone %q is %s", zone.GetName(), zone.GetStatus())
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

func (env *environ) instancePlacementZone(placement, volumeAttachmentsZone string) (string, error) {
	instPlacement, err := env.parsePlacement(placement)
	if err != nil {
		return "", errors.Trace(err)
	}
	if instPlacement.zone == "" {
		return volumeAttachmentsZone, nil
	}
	zoneName := instPlacement.zone
	if volumeAttachmentsZone != "" && volumeAttachmentsZone != zoneName {
		return "", errors.Errorf(
			"cannot create instance in zone %q, as this will prevent attaching the requested disks in zone %q",
			zoneName, volumeAttachmentsZone,
		)
	}
	return zoneName, nil
}
