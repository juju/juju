// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"strings"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

type vmwareAvailZone struct {
	r    mo.ComputeResource
	pool *object.ResourcePool
	name string
}

// Name returns the "name" of the Vsphere availability zone.
func (z *vmwareAvailZone) Name() string {
	return z.name
}

// Available implements common.AvailabilityZone
func (z *vmwareAvailZone) Available() bool {
	return true
}

// AvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) AvailabilityZones(ctx context.ProviderCallContext) (zones network.AvailabilityZones, err error) {
	err = env.withSession(ctx, func(env *sessionEnviron) error {
		zones, err = env.AvailabilityZones(ctx)
		return err
	})
	return zones, err
}

// AvailabilityZones is part of the common.ZonedEnviron interface.
func (env *sessionEnviron) AvailabilityZones(ctx context.ProviderCallContext) (network.AvailabilityZones, error) {
	if len(env.zones) > 0 {
		// This is relatively expensive to compute, so cache it on the session
		return env.zones, nil
	}

	folders, err := env.client.Folders(env.ctx)
	if err != nil {
		HandleCredentialError(err, env, ctx)
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
	var zones network.AvailabilityZones
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
				r:    *cr.Resource,
				pool: pool,
				name: makeAvailZoneName(hostFolder, cr.Path, pool.InventoryPath),
			}
			logger.Tracef("zone: %s (cr.Name=%q pool.InventoryPath=%q)",
				zone.Name(), zone.r.Name, zone.pool.InventoryPath)
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

// makeAvailZoneName constructs a Vsphere availability zone name from the
// given paths. Basically it's the path relative to the host folder without
// the extra "Resources" path segment (which doesn't appear in the UI).
func makeAvailZoneName(hostFolder, crPath, poolPath string) string {
	poolPath = strings.TrimRight(poolPath, "/")
	relCrPath := strings.TrimPrefix(crPath, hostFolder+"/")
	relPoolPath := strings.TrimPrefix(poolPath, crPath+"/")
	switch {
	case relPoolPath == "Resources":
		// "/DataCenter1/host/Host1/Resources" becomes "Host1"
		return relCrPath
	case strings.HasPrefix(relPoolPath, "Resources/"):
		// "/DataCenter1/host/Host1/Resources/ResPool1" becomes "Host1/ResPool1"
		return relCrPath + "/" + relPoolPath[len("Resources/"):]
	default:
		// "/DataCenter1/host/Host1/Other" becomes "Host1/Other" (shouldn't happen)
		return relCrPath + "/" + relPoolPath
	}
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
		if z.Name() == name {
			return z.(*vmwareAvailZone), nil
		}
	}
	return nil, errors.NotFoundf("availability zone %q", name)
}
