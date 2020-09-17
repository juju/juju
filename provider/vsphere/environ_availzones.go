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
	rPath      string
	pool       *object.ResourcePool
	hostFolder string
}

// Name implements common.AvailabilityZone
func (z *vmwareAvailZone) Name() string {
	// Strip "/DataCenter1/host/" prefix from compute resource or resource pool path
	return strings.TrimPrefix(z.path(), z.hostFolder+"/")
}

// Available implements common.AvailabilityZone
func (z *vmwareAvailZone) Available() bool {
	return true
}

func (z *vmwareAvailZone) path() string {
	if z.pool == nil {
		return z.rPath
	}
	return z.pool.InventoryPath
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

	computeResources, crPaths, err := env.client.ComputeResources(env.ctx)
	if err != nil {
		HandleCredentialError(err, env, ctx)
		return nil, errors.Trace(err)
	}
	var zones []common.AvailabilityZone
	for i, cr := range computeResources {
		if cr.Summary.GetComputeResourceSummary().EffectiveCpu == 0 {
			logger.Debugf("skipping empty compute resource %q", cr.Name)
			continue
		}

		// Add an availability zone for this compute resource directly, eg:
		// "/DataCenter1/host/Host1"
		zone := &vmwareAvailZone{
			r:          *cr,
			rPath:      crPaths[i],
			hostFolder: hostFolder,
		}
		logger.Tracef("zone from compute resource: %s (cr.Name=%q rPath=%q hostFolder=%q)",
			zone.Name(), zone.r.Name, zone.rPath, zone.hostFolder)
		zones = append(zones, zone)

		// Then add an availability zone for each resource pool under this
		// compute resource, eg: "/DataCenter1/host/Host1/Resources"
		pools, err := env.client.ResourcePools(env.ctx, crPaths[i]+"/...")
		if err != nil {
			HandleCredentialError(err, env, ctx)
			return nil, errors.Trace(err)
		}
		for _, pool := range pools {
			zone = &vmwareAvailZone{
				r:          *cr,
				rPath:      crPaths[i],
				pool:       pool,
				hostFolder: hostFolder,
			}
			logger.Tracef("zone from resource pool: %s (cr.Name=%q rPath=%q pool.InventoryPath=%q hostFolder=%q)",
				zone.Name(), zone.r.Name, zone.rPath, zone.pool.InventoryPath, zone.hostFolder)
			zones = append(zones, zone)
		}
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
			if pool == nil {
				// Skip availability zones that aren't resource pools
				continue
			}
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
// cluster, or resource pool), allow match on absolute path, path relative to
// host folder, or legacy resource pool match.
func (env *environ) ZoneMatches(zone, constraint string) bool {
	return zoneMatches(zone, constraint)
}

func zoneMatches(zone, constraint string) bool {
	// If they've specified an absolute path, strip the datacenter/host
	// segments; for example "/DataCenter1/host/Cluster1/Host1"
	// becomes "Cluster1/Host1". This allows the user to specify an
	// absolute path, like those from "govc find".
	//
	// TODO benhoyt: maybe we don't want absolute path matching at all,
	// because if there's a datacenter folder we don't know how deep it is,
	// so this will fail.
	if strings.HasPrefix(constraint, "/") {
		parts := strings.Split(constraint, "/")
		// Must be at least ["" "DataCenter1" "host" "Cluster1"]
		if len(parts) < 4 {
			return false
		}
		constraint = strings.Join(parts[3:], "/")
	}

	// Allow match on full zone name (path without host folder prefix), for
	// example "Cluster1/Host1".
	if zone == constraint {
		return true
	}

	// Otherwise, for resource pools, allow them to omit the "Resources"
	// part of the path (for backwards compatibility). For example, for pool
	// "Host1/Resources/Parent/Child", allow a match on "Host1/Parent/Child".
	//
	// TODO: should we consider stripping "Resources" at any level? That may help with folders
	parts := strings.Split(zone, "/")
	if len(parts) > 2 && parts[1] == "Resources" {
		legacyZone := parts[0] + "/" + strings.Join(parts[2:], "/")
		if legacyZone == constraint {
			return true
		}
	}
	return false
}
