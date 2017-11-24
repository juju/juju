// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import "github.com/juju/errors"

// ZoneIndependentError wraps the given error such that it
// satisfies environs.IsAvailabilityZoneIndependent.
func ZoneIndependentError(err error) error {
	if err == nil {
		return nil
	}
	wrapped := errors.Wrap(err, zoneIndependentError{err})
	wrapped.(*errors.Err).SetLocation(1)
	return wrapped
}

type zoneIndependentError struct {
	error
}

// AvailabilityZoneIndependent is part of the
// environs.AvailabilityZoneError interface.
func (zoneIndependentError) AvailabilityZoneIndependent() bool {
	return true
}
