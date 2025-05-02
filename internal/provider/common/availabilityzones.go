// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"sort"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

// ZonedEnviron is an environs.Environ that has support for availability zones.
type ZonedEnviron interface {
	environs.Environ

	// AvailabilityZones returns all availability zones in the environment.
	AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error)

	// InstanceAvailabilityZoneNames returns the names of the availability
	// zones for the specified instances. The error returned follows the same
	// rules as Environ.Instances.
	InstanceAvailabilityZoneNames(ctx context.Context, ids []instance.Id) (map[instance.Id]string, error)

	// DeriveAvailabilityZones attempts to derive availability zones from
	// the specified StartInstanceParams.
	//
	// The parameters for starting an instance may imply (or explicitly
	// specify) availability zones, e.g. due to placement, or due to the
	// attachment of existing volumes, or due to subnet placement. If
	// there is no such restriction, then DeriveAvailabilityZones should
	// return an empty string slice to indicate that the caller should
	// choose an availability zone.
	DeriveAvailabilityZones(ctx context.Context, args environs.StartInstanceParams) ([]string, error)
}

// AvailabilityZoneInstances describes an availability zone and
// a set of instances in that zone.
type AvailabilityZoneInstances struct {
	// ZoneName is the name of the availability zone.
	ZoneName string

	// Instances is a set of instances within the availability zone.
	Instances []instance.Id
}

type byPopulationThenName []AvailabilityZoneInstances

func (b byPopulationThenName) Len() int {
	return len(b)
}

func (b byPopulationThenName) Less(i, j int) bool {
	switch {
	case len(b[i].Instances) < len(b[j].Instances):
		return true
	case len(b[i].Instances) == len(b[j].Instances):
		return b[i].ZoneName < b[j].ZoneName
	}
	return false
}

func (b byPopulationThenName) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

// AvailabilityZoneAllocations returns the availability zones and their
// instance allocations from the specified group, in ascending order of
// population. Availability zones with the same population size are
// ordered by name.
//
// If the specified group is empty, then it will behave as if the result of
// AllRunningInstances were provided.
func AvailabilityZoneAllocations(
	env ZonedEnviron, ctx context.Context, group []instance.Id,
) ([]AvailabilityZoneInstances, error) {
	if len(group) == 0 {
		instances, err := env.AllRunningInstances(ctx)
		if err != nil {
			return nil, err
		}
		group = make([]instance.Id, len(instances))
		for i, inst := range instances {
			group[i] = inst.Id()
		}
	}
	instanceZones, err := env.InstanceAvailabilityZoneNames(ctx, group)
	switch err {
	case nil, environs.ErrPartialInstances:
	case environs.ErrNoInstances:
		group = nil
	default:
		return nil, err
	}

	// Get the list of all "available" availability zones,
	// and then initialise a tally for each one.
	zones, err := env.AvailabilityZones(ctx)
	if err != nil {
		return nil, err
	}
	instancesByZoneName := make(map[string][]instance.Id)
	for _, zone := range zones {
		if !zone.Available() {
			continue
		}
		name := zone.Name()
		instancesByZoneName[name] = nil
	}
	if len(instancesByZoneName) == 0 {
		return nil, nil
	}

	for _, id := range group {
		zone := instanceZones[id]
		if zone == "" {
			continue
		}
		if _, ok := instancesByZoneName[zone]; !ok {
			// zone is not available
			continue
		}
		instancesByZoneName[zone] = append(instancesByZoneName[zone], id)
	}

	zoneInstances := make([]AvailabilityZoneInstances, 0, len(instancesByZoneName))
	for zoneName, instances := range instancesByZoneName {
		zoneInstances = append(zoneInstances, AvailabilityZoneInstances{
			ZoneName:  zoneName,
			Instances: instances,
		})
	}
	sort.Sort(byPopulationThenName(zoneInstances))
	return zoneInstances, nil
}
