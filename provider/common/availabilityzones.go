// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"sort"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
)

// AvailabilityZone describes a provider availability zone.
type AvailabilityZone interface {
	// Name returns the name of the availability zone.
	Name() string

	// Available reports whether the availability zone is currently available.
	Available() bool
}

// ZonedEnviron is an environs.Environ that has support for
// availability zones.
type ZonedEnviron interface {
	environs.Environ

	// AvailabilityZones returns all availability zones in the environment.
	AvailabilityZones() ([]AvailabilityZone, error)

	// InstanceAvailabilityZoneNames returns the names of the availability
	// zones for the specified instances. The error returned follows the same
	// rules as Environ.Instances.
	InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error)
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
// AllInstances were provided.
func AvailabilityZoneAllocations(env ZonedEnviron, group []instance.Id) ([]AvailabilityZoneInstances, error) {
	if len(group) == 0 {
		instances, err := env.AllInstances()
		if err != nil {
			return nil, err
		}
		group = make([]instance.Id, len(instances))
		for i, inst := range instances {
			group[i] = inst.Id()
		}
	}
	instanceZones, err := env.InstanceAvailabilityZoneNames(group)
	switch err {
	case nil, environs.ErrPartialInstances:
	case environs.ErrNoInstances:
		group = nil
	default:
		return nil, err
	}

	// Get the list of all "available" availability zones,
	// and then initialise a tally for each one.
	zones, err := env.AvailabilityZones()
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

	for i, id := range group {
		zone := instanceZones[i]
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

var internalAvailabilityZoneAllocations = AvailabilityZoneAllocations

// DistributeInstances is a common function for implement the
// state.InstanceDistributor policy based on availability zone
// spread.
func DistributeInstances(env ZonedEnviron, candidates, group []instance.Id) ([]instance.Id, error) {
	// Determine the best availability zones for the group.
	zoneInstances, err := internalAvailabilityZoneAllocations(env, group)
	if err != nil || len(zoneInstances) == 0 {
		return nil, err
	}

	// Determine which of the candidates are eligible based on whether
	// they are allocated in one of the best availability zones.
	var allEligible []string
	for i := range zoneInstances {
		if i > 0 && len(zoneInstances[i].Instances) > len(zoneInstances[i-1].Instances) {
			break
		}
		for _, id := range zoneInstances[i].Instances {
			allEligible = append(allEligible, string(id))
		}
	}
	sort.Strings(allEligible)
	eligible := make([]instance.Id, 0, len(candidates))
	for _, candidate := range candidates {
		n := sort.SearchStrings(allEligible, string(candidate))
		if n >= 0 && n < len(allEligible) {
			eligible = append(eligible, candidate)
		}
	}
	return eligible, nil
}
