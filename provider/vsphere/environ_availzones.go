// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"strings"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

type vmwareAvailZone struct {
	r          mo.ComputeResource
	pool       *object.ResourcePool
	hostFolder string
}

// Name returns the "name" of the Vsphere availability zone, which is the path
// relative to the datacenter's host folder.
func (z *vmwareAvailZone) Name() string {
	// Strip "/DataCenter1/host/" prefix from resource pool path
	return strings.TrimPrefix(z.pool.InventoryPath, z.hostFolder+"/")
}

// Available implements common.AvailabilityZone
func (z *vmwareAvailZone) Available() bool {
	return true
}

// AvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) AvailabilityZones(ctx context.ProviderCallContext) (zones []common.AvailabilityZone, err error) {
	err = env.withSession(ctx, func(env *sessionEnviron) error {
		zones, err = env.AvailabilityZones(ctx)
		return err
	})
	return zones, err
}

// AvailabilityZones is part of the common.ZonedEnviron interface.
func (env *sessionEnviron) AvailabilityZones(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
	if len(env.zones) > 0 {
		// This is relatively expensive to compute, so cache it on the session
		return env.zones, nil
	}

	folders, err := env.client.Folders(env.ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Tracef("host folder InventoryPath=%q, Name=%q",
		folders.HostFolder.InventoryPath, folders.HostFolder.Name())
	hostFolder := folders.HostFolder.InventoryPath

	computeResources, err := env.client.ComputeResources(env.ctx)
	if err != nil {
		HandleCredentialError(err, env, ctx)
		return nil, errors.Trace(err)
	}
	var zones []common.AvailabilityZone
	for _, cr := range computeResources {
		if cr.Resource.Summary.GetComputeResourceSummary().EffectiveCpu == 0 {
			logger.Debugf("skipping empty compute resource %q", cr.Resource.Name)
			continue
		}

		// Add an availability zone for each resource pool under this compute
		// resource, eg: "/DataCenter1/host/Host1/Resources"
		pools, err := env.client.ResourcePools(env.ctx, cr.Path+"/...")
		if err != nil {
			HandleCredentialError(err, env, ctx)
			return nil, errors.Trace(err)
		}
		for _, pool := range pools {
			zone := &vmwareAvailZone{
				r:          *cr.Resource,
				pool:       pool,
				hostFolder: hostFolder,
			}
			logger.Tracef("zone: %s (cr.Name=%q pool.InventoryPath=%q hostFolder=%q)",
				zone.Name(), zone.r.Name, zone.pool.InventoryPath, zone.hostFolder)
			zones = append(zones, zone)
		}
	}

	if logger.IsDebugEnabled() {
		zoneNames := make([]string, len(zones))
		for i, zone := range zones {
			zoneNames[i] = zone.Name()
		}
		logger.Debugf("fetched availability zones: %q", zoneNames)
	}

	env.zones = zones
	return env.zones, nil
}

// InstanceAvailabilityZoneNames is part of the common.ZonedEnviron interface.
func (env *environ) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) (names []string, err error) {
	err = env.withSession(ctx, func(env *sessionEnviron) error {
		names, err = env.InstanceAvailabilityZoneNames(ctx, ids)
		return err
	})
	return names, err
}

// InstanceAvailabilityZoneNames is part of the common.ZonedEnviron interface.
func (env *sessionEnviron) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
	zones, err := env.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	instances, err := env.Instances(ctx, ids)
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
			pool := zone.(*vmwareAvailZone).pool
			if pool.Reference().Value == vm.ResourcePool.Value {
				results[i] = zone.Name()
				break
			}
		}
	}
	return results, err
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) (names []string, err error) {
	err = env.withSession(ctx, func(env *sessionEnviron) error {
		names, err = env.DeriveAvailabilityZones(ctx, args)
		return err
	})
	return names, err
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *sessionEnviron) DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	if args.Placement != "" {
		// args.Placement will always be a zone name or empty.
		placement, err := env.parsePlacement(ctx, args.Placement)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if placement.Name() != "" {
			return []string{placement.Name()}, nil
		}
	}
	return nil, nil
}

func (env *sessionEnviron) availZone(ctx context.ProviderCallContext, name string) (*vmwareAvailZone, error) {
	zones, err := env.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, z := range zones {
		if env.ZoneMatches(z.Name(), name) {
			return z.(*vmwareAvailZone), nil
		}
	}
	return nil, errors.NotFoundf("availability zone %q", name)
}

// ZoneMatches implements zoneMatcher interface to allow custom matching (see
// provider/common.ZoneMatches). For a Vsphere "availability zone" (host,
// cluster, or resource pool), allow a match on the path relative to the host
// folder, or a legacy resource pool match.
func (env *environ) ZoneMatches(zone, constraint string) bool {
	// Allow match on full zone name (path without host folder prefix), for
	// example "Cluster1/Host1".
	if zone == constraint {
		return true
	}

	// Otherwise allow them to omit the "Resources" part of the path (for
	// backwards compatibility). For example, for pool
	// "Host1/Resources/Parent/Child", allow a match on "Host1/Parent/Child".
	parts := strings.Split(zone, "/")
	var partsWithoutResources []string
	for _, part := range parts {
		if part != "Resources" {
			partsWithoutResources = append(partsWithoutResources, part)
		}
	}
	legacyZone := strings.Join(partsWithoutResources, "/")
	return legacyZone == constraint
}
