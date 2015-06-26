// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"google.golang.org/api/compute/v1"
)

// AvailabilityZone represents a single GCE zone. It satisfies the
// {provider/common}.AvailabilityZone interface.
type AvailabilityZone struct {
	zone *compute.Zone
}

// NewZone build an availability zone from the provided name, status
// state, and replacement and returns it.
func NewZone(name, status, state, replacement string) AvailabilityZone {
	zone := &compute.Zone{
		Name:   name,
		Status: status,
	}
	if state != "" {
		zone.Deprecated = &compute.DeprecationStatus{
			State:       state,
			Replacement: replacement,
		}
	}
	return AvailabilityZone{zone: zone}
}

// TODO(ericsnow) Add a Region getter?

// Name returns the zone's name.
func (z AvailabilityZone) Name() string {
	return z.zone.Name
}

// Status returns the status string for the zone. It will match one of
// the Status* constants defined in the package.
func (z AvailabilityZone) Status() string {
	return z.zone.Status
}

// Deprecated returns true if the zone has been deprecated.
func (z AvailabilityZone) Deprecated() bool {
	deprecated := z.zone.Deprecated != nil
	if deprecated {
		logger.Warningf("zone %q is %q", z.Name(), z.zone.Deprecated.State)
		if z.zone.Deprecated.Replacement != "" {
			logger.Warningf("zone %q is the replacement for zone %q", z.zone.Deprecated.Replacement, z.Name())
		}
	}
	return deprecated
}

// Available returns whether or not the zone is available for provisioning.
func (z AvailabilityZone) Available() bool {
	// https://cloud.google.com/compute/docs/reference/latest/zones#status
	return z.Status() == StatusUp
}
