// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// AvailabilityZone type used to hold
// the zone that is available for the oracle provider
type AvailabilityZone struct {
	// name is the nam of the zone
	name string
}

// NewAvailabilityZone returns a new availability zone with the given
// name zone provided
func NewAvailabilityZone(name string) AvailabilityZone {
	return AvailabilityZone{
		name: name,
	}
}

// Name returns the name zone
func (a AvailabilityZone) Name() string {
	return a.name
}

// Available returns true if the availability zone is available
func (a AvailabilityZone) Available() bool {
	// we don't really have availability zones in oracle cloud. We only
	// have regions
	return true
}
