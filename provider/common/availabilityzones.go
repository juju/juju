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

// BestAvailabilityZoneAllocations returns the availability zones with the
// fewest instances from the specified group, along with the instances from
// the group currently allocated to those zones.
//
// If the specified group is empty, then it will behave as if the result of
// AllInstances were provided.
func BestAvailabilityZoneAllocations(env ZonedEnviron, group []instance.Id) (map[string][]instance.Id, error) {
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
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}

	// Get the list of all "available" availability zones,
	// and then initialise a tally for each one.
	zones, err := env.AvailabilityZones()
	if err != nil {
		return nil, err
	}
	if len(zones) == 0 {
		return nil, nil
	}
	zoneTally := make(map[string]int, len(zones))
	for _, zone := range zones {
		if !zone.Available() {
			continue
		}
		zoneTally[zone.Name()] = 0
	}

	// Tally the zones with provisioned instances and return
	// the zones with equal fewest instances from the group.
	zoneInstances := make(map[string][]instance.Id)
	for zone := range zoneTally {
		// Initialise each zone's instance slice to nil,
		// in case there are zones without instances.
		zoneInstances[zone] = nil
	}
	for i, id := range group {
		zone := instanceZones[i]
		if zone == "" {
			continue
		}
		if _, ok := zoneTally[zone]; !ok {
			// zone is not available
			continue
		}
		zoneTally[zone] += 1
		zoneInstances[zone] = append(zoneInstances[zone], id)
	}
	minZoneTally := -1
	for _, tally := range zoneTally {
		if minZoneTally == -1 || tally < minZoneTally {
			minZoneTally = tally
		}
	}
	for zone, tally := range zoneTally {
		if tally > minZoneTally {
			delete(zoneInstances, zone)
		}
	}
	return zoneInstances, nil
}

var internalBestAvailabilityZoneAllocations = BestAvailabilityZoneAllocations

// DistributeInstances is a common function for implement the
// state.InstanceDistributor policy based on availability zone
// spread.
func DistributeInstances(env ZonedEnviron, candidates, group []instance.Id) ([]instance.Id, error) {
	// Determine the best availability zones for the group.
	bestAvailabilityZones, err := internalBestAvailabilityZoneAllocations(env, group)
	if err != nil {
		return nil, err
	}

	// Determine which of the candidates are eligible based on whether
	// they are allocated in one of the best availability zones.
	var allEligible []string
	for _, ids := range bestAvailabilityZones {
		for _, id := range ids {
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
