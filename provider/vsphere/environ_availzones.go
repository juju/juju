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

// poolPathPrefixParts is the number of path components to chop off a
// resource pool's path to get its availability zone path. The paths
// look like:
// /<datacenter name>/host/<compute resource name>/Resources/<pool path...>
// So there are 5 parts to the prefix (counting the
// blank one from the leading /).
const poolPathPrefixParts = 5

type vmwareAvailZone struct {
	r    mo.ComputeResource
	pool *object.ResourcePool
}

// Name implements common.AvailabilityZone
func (z *vmwareAvailZone) Name() string {
	// The name for this zone is the compute resource name and the
	// path of the pool without the prefix, so for
	// /QA/host/aron.internal/Resources/High/Child, the name should be
	// aron.internal/High/Child.
	path := strings.TrimRight(z.pool.InventoryPath, "/")
	parts := strings.Split(path, "/")
	poolPath := ""
	if len(parts) > poolPathPrefixParts {
		// This isn't the root pool for this compute resource, include
		// the pool's path.
		poolPath = "/" + strings.Join(parts[poolPathPrefixParts:], "/")
	}

	return z.r.Name + poolPath
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
	logger.Debugf("LP #1894236: AvailabilityZones() env.zones: %+v", env.zones)

	// if env.zones == nil {  // LP #1894236
	if len(env.zones) == 0 {
		computeResources, err := env.client.ComputeResources(env.ctx)
		logger.Debugf("LP #1894236: AvailabilityZones() computeResources: %+v", computeResources)
		if err != nil {
			HandleCredentialError(err, env, ctx)
			return nil, errors.Trace(err)
		}
		var zones network.AvailabilityZones
		for _, cr := range computeResources {
			if cr.Summary.GetComputeResourceSummary().EffectiveCpu == 0 {
				logger.Debugf("skipping empty compute resource %q", cr.Name)
				continue
			}

			logger.Debugf("LP #1894236: AvailabilityZones() cr.Name: %v", cr.Name)
			logger.Debugf("LP #1894236: AvailabilityZones() cr.Parent: %+v", cr.Parent)

			if "Folder" == cr.Parent.Type {
				logger.Debugf("LP #1894236: AvailabilityZones() cr.Parent.Type == 'Folder'")
				// TODO: retrieve folder's full path, merge it with cr.Name  and pass it to
				// ResourcePools() below

				// folderFullPath = TODO
				// path = folderFullPath+"/"+cr.Name+"/..."
			}

			pools, err := env.client.ResourcePools(env.ctx, cr.Name+"/...")
			logger.Debugf("LP #1894236: AvailabilityZones() pools: %+v", pools)
			if err != nil {
				HandleCredentialError(err, env, ctx)
				return nil, errors.Trace(err)
			}
			for _, pool := range pools {
				zone := &vmwareAvailZone{
					r:    *cr,
					pool: pool,
				}
				logger.Tracef("zone: %q", zone.Name())
				zones = append(zones, zone)
			}
		}
		logger.Debugf("LP #1894236: AvailabilityZones() zones: %+v", zones)
		env.zones = zones
	}
	logger.Debugf("LP #1894236: AvailabilityZones() return value: %+v", env.zones)
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
		if z.Name() == name {
			return z.(*vmwareAvailZone), nil
		}
	}
	return nil, errors.NotFoundf("availability zone %q", name)
}
