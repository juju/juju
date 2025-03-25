// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// AvailabilityZone describes the common methods
// for general interaction with an AZ.
type AvailabilityZone interface {
	// Name returns the name of the availability zone.
	Name() string

	// Available reports whether the availability zone is currently available.
	Available() bool
}

// AvailabilityZones is a collection of AvailabilityZone.
type AvailabilityZones []AvailabilityZone

// Validate checks that a zone with the input name exists and is available
// according to the topology represented by the receiver.
// An error is returned if either of these conditions are not met.
func (a AvailabilityZones) Validate(zoneName string) error {
	for _, az := range a {
		if az.Name() == zoneName {
			if az.Available() {
				return nil
			}
			return errors.Errorf("zone %q is unavailable", zoneName)
		}
	}
	return errors.Errorf("availability zone %q %w", zoneName, coreerrors.NotValid)
}
