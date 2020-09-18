// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"sort"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// AvailabilityZone describes a provider availability zone.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/availability_zone.go github.com/juju/juju/provider/common AvailabilityZone
type AvailabilityZone interface {
	// Name returns the name of the availability zone.
	Name() string

	// Available reports whether the availability zone is currently available.
	Available() bool
}

// ZonedEnviron is an environs.Environ that has support for availability zones.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/zoned_environ.go github.com/juju/juju/provider/common ZonedEnviron
type ZonedEnviron interface {
	environs.Environ

	// AvailabilityZones returns all availability zones in the environment.
	AvailabilityZones(ctx context.ProviderCallContext) ([]AvailabilityZone, error)

	// InstanceAvailabilityZoneNames returns the names of the availability
	// zones for the specified instances. The error returned follows the same
	// rules as Environ.Instances.
	InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error)

	// DeriveAvailabilityZones attempts to derive availability zones from
	// the specified StartInstanceParams.
	//
	// The parameters for starting an instance may imply (or explicitly
	// specify) availability zones, e.g. due to placement, or due to the
	// attachment of existing volumes, or due to subnet placement. If
	// there is no such restriction, then DeriveAvailabilityZones should
	// return an empty string slice to indicate that the caller should
	// choose an availability zone.
	DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error)
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
	env ZonedEnviron, ctx context.ProviderCallContext, group []instance.Id,
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
// state.InstanceDistributor policy based on availability zone spread.
// TODO (manadart 2018-11-27) This method signature has grown to the point
// where the argument list should be replaced with a struct.
// At that time limitZones could be transformed to a map so that lookups in the
// filtering below are more efficient.
func DistributeInstances(
	env ZonedEnviron, ctx context.ProviderCallContext, candidates, group []instance.Id, limitZones []string,
) ([]instance.Id, error) {
	// Determine availability zone distribution for the group.
	zoneInstances, err := internalAvailabilityZoneAllocations(env, ctx, group)
	if err != nil || len(zoneInstances) == 0 {
		return nil, err
	}

	// If there any zones supplied for limitation,
	// filter to distribution data so that only those zones are considered.
	filteredZoneInstances := zoneInstances[:0]
	if len(limitZones) > 0 {
		for _, zi := range zoneInstances {
			for _, zone := range limitZones {
				if zi.ZoneName == zone {
					filteredZoneInstances = append(filteredZoneInstances, zi)
					break
				}
			}
		}
	} else {
		filteredZoneInstances = zoneInstances
	}

	// Determine which of the candidates are eligible based on whether
	// they are allocated in one of the least-populated availability zones.
	var allEligible []string
	for i := range filteredZoneInstances {
		if i > 0 && len(filteredZoneInstances[i].Instances) > len(filteredZoneInstances[i-1].Instances) {
			break
		}
		for _, id := range filteredZoneInstances[i].Instances {
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

// ValidateAvailabilityZone returns nil iff the availability
// zone exists and is available, otherwise returns a NotValid
// error.
func ValidateAvailabilityZone(env ZonedEnviron, ctx context.ProviderCallContext, zone string) error {
	zones, err := env.AvailabilityZones(ctx)
	if err != nil {
		return err
	}
	for _, z := range zones {
		if z.Name() == zone {
			if z.Available() {
				return nil
			}
			return errors.Errorf("availability zone %q is unavailable", zone)
		}
	}
	return errors.NotValidf("availability zone %q", zone)
}
