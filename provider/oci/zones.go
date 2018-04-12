// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

// AvailabilityZone implements common.AvailabilityZone
type AvailabilityZone struct {
	// name is the name of the zone
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
	return true
}
