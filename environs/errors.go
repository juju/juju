// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"
)

var (
	ErrNotBootstrapped  = errors.New("model is not bootstrapped")
	ErrNoInstances      = errors.NotFoundf("instances")
	ErrPartialInstances = errors.New("only some instances were found")
)

// AvailabilityZoneError provides an interface for compute providers
// to indicate whether or not an error is specific to, or independent
// of, any particular availability zone.
type AvailabilityZoneError interface {
	error

	// AvailabilityZoneIndependent reports whether or not the
	// error is related to a specific availability zone.
	AvailabilityZoneIndependent() bool
}

// IsAvailabilityZoneIndependent reports whether or not the given error,
// or its cause, is independent of any particular availability zone.
// Juju uses this to decide whether or not to attempt the failed operation
// in another availability zone; zone-independent failures will not be
// reattempted.
//
// If the error implements AvailabilityZoneError, then the result of
// calling its AvailabilityZoneIndependent method will be returned;
// otherwise this function returns false. That is, errors are assumed
// to be specific to an availability zone by default, so that they can
// be retried in another availability zone.
func IsAvailabilityZoneIndependent(err error) bool {
	if err, ok := errors.Cause(err).(AvailabilityZoneError); ok {
		return err.AvailabilityZoneIndependent()
	}
	return false
}
