// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// AvailabilityZone implements common.AvailabilityZone
type AvailabilityZone struct {
	// name is the nam of the zone
	name string
}

// NewAvailabilityZone returns a new availability zone
func NewAvailabilityZone(name string) AvailabilityZone {
	return AvailabilityZone{
		name: name,
	}
}

// Name is specified on the common.AvailabilityZone interface
func (a AvailabilityZone) Name() string {
	return a.name
}

// Available is specified on the common.AvailabilityZone interface
func (a AvailabilityZone) Available() bool {
	// we don't really have availability zones in oracle cloud. We only
	// have regions
	return true
}
