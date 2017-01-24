// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
)

var _ environs.InstanceTypesFetcher = (*environ)(nil)
var virtType = "kvm"

// InstanceTypes implements InstanceTypesFetcher
func (env *environ) InstanceTypes(c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	reg, err := env.Region()
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	zones, err := env.gce.AvailabilityZones(reg.Region)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	resultUnique := map[string]instances.InstanceType{}

	for _, z := range zones {
		if !z.Available() {
			continue
		}
		machines, err := env.gce.ListMachineTypes(z.Name())
		if err != nil {
			return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
		}
		for _, m := range machines {
			i := instances.InstanceType{
				Id:       strconv.FormatUint(m.Id, 10),
				Name:     m.Name,
				CpuCores: uint64(m.GuestCpus),
				Mem:      uint64(m.MemoryMb),
				Arches:   []string{arch.AMD64},
				VirtType: &virtType,
			}
			resultUnique[m.Name] = i
		}
	}

	result := make([]instances.InstanceType, len(resultUnique))
	i := 0
	for _, it := range resultUnique {
		result[i] = it
		i++
	}
	result, err = instances.MatchingInstanceTypes(result, "", c)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	return instances.InstanceTypesWithCostMetadata{InstanceTypes: result}, nil
}
