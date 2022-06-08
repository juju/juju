// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

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

// PreferredStorageNotFound is an error that indicates to the caller the environ
// was unable to completes it's operation because it could not find the storage
// it prefers for the operation. i.e aws block storage or Kubernetes cluster
// storage.
type PreferredStorageNotFound struct {
	Message string
}

// NominatedStorageNotFound is an error that indicates the storage the user
// nominated to use for a specific operation was not found and needs to be
// checked for existence or typo's.
type NominatedStorageNotFound struct {
	StorageName string
}

// Error implements the error interface
func (p *PreferredStorageNotFound) Error() string {
	return p.Message
}

// Error implements the error interface
func (n *NominatedStorageNotFound) Error() string {
	return fmt.Sprintf("nominated storage %q not found", n.StorageName)
}
