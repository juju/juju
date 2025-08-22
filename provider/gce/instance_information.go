// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"strconv"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/gce/internal/google"
)

var (
	_ environs.InstanceTypesFetcher = (*environ)(nil)

	virtType = "kvm"

	// minCpuCores is the assumed minimum CPU cores we prefer in order to run a server.
	minCpuCores uint64 = 2
)

// ensureDefaultConstraints adds the minimum number of cpu cores value to the
// constraints if the user has not provided a cpu-cores constraint.
// This function exists only so that the minimum cpu cores takes precedence
// over the default cpu-cores and memory values implemented in
// instances.MatchingInstanceTypes()
func ensureDefaultConstraints(c constraints.Value) constraints.Value {
	if c.HasInstanceType() || c.HasCpuCores() {
		return c
	}
	c.CpuCores = &minCpuCores
	return c
}

// InstanceTypes implements InstanceTypesFetcher
func (env *environ) InstanceTypes(ctx context.ProviderCallContext, c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	allInstanceTypes, err := env.getAllInstanceTypes(ctx, clock.WallClock)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	matches, err := instances.MatchingInstanceTypes(allInstanceTypes, "", ensureDefaultConstraints(c))
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	return instances.InstanceTypesWithCostMetadata{InstanceTypes: matches}, nil
}

// getAllInstanceTypes fetches and memoizes the list of available GCE instances
// for the AZs associated with the current region.
func (env *environ) getAllInstanceTypes(ctx context.ProviderCallContext, clock clock.Clock) ([]instances.InstanceType, error) {
	env.instTypeListLock.Lock()
	defer env.instTypeListLock.Unlock()

	if len(env.cachedInstanceTypes) != 0 && clock.Now().Before(env.instCacheExpireAt) {
		return env.cachedInstanceTypes, nil
	}

	reg, err := env.Region()
	if err != nil {
		return nil, errors.Trace(err)
	}
	zones, err := env.gce.AvailabilityZones(ctx, reg.Region)
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}
	resultUnique := map[string]instances.InstanceType{}

	for _, z := range zones {
		if z.Status != google.StatusUp {
			continue
		}
		machines, err := env.gce.ListMachineTypes(ctx, z.Name)
		if err != nil {
			return nil, google.HandleCredentialError(errors.Trace(err), ctx)
		}
		for _, m := range machines {
			i := instances.InstanceType{
				Id:       strconv.FormatUint(m.Id, 10),
				Name:     m.Name,
				CpuCores: uint64(m.GuestCpus),
				Mem:      uint64(m.MemoryMb),
				// TODO: support arm64 once the API can report arch.
				Arch:     arch.AMD64,
				VirtType: &virtType,
			}
			resultUnique[m.Name] = i
		}
	}

	env.cachedInstanceTypes = make([]instances.InstanceType, 0, len(resultUnique))
	for _, it := range resultUnique {
		env.cachedInstanceTypes = append(env.cachedInstanceTypes, it)
	}

	// Keep the instance data in the cache for 10 minutes. This is probably
	// long enough to exploit temporal locality when deploying bundles etc
	// and short enough to allow the use of new machines a few moments after
	// they are published by the GCE.
	env.instCacheExpireAt = clock.Now().Add(10 * time.Minute)
	return env.cachedInstanceTypes, nil
}
