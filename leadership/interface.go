// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"
)

// LeadershipClaimDeniedErr is the error which will be returned when a
// leadership claim has been denied.
var LeadershipClaimDeniedErr = errors.New("leadership claim denied")

type LeadershipManager interface {
	// ClaimLeadership claims a leadership for the given serviceId and
	// unitId. If successful, the duration of the leadership lease is
	// returned.
	ClaimLeadership(serviceId, unitId string) (nextClaimInterval time.Duration, err error)
	// ReleaseLeadership releases a leadership claim for the given
	// serviceId and unitId.
	ReleaseLeadership(serviceId, unitId string) (err error)
	// BlockUntilLeadershipReleased blocks the caller until leadership is
	// released for the given serviceId.
	BlockUntilLeadershipReleased(serviceId string) (err error)
}

type LeadershipLeaseManager interface {
	// Claimlease claims a lease for the given duration for the given
	// namespace and id. If the lease is already owned, a
	// LeaseClaimDeniedErr will be returned. Either way the current lease
	// owner's ID will be returned.
	ClaimLease(namespace, id string, forDur time.Duration) (leaseOwnerId string, err error)
	// ReleaseLease releases the lease held for namespace by id.
	ReleaseLease(namespace, id string) (err error)
	// LeaseReleasedNotifier returns a channel a caller can block on to be
	// notified of when a lease is released for namespace. This channel is
	// reusable, but will be closed if it does not respond within
	// "notificationTimeout".
	LeaseReleasedNotifier(namespace string) (notifier <-chan struct{})
}
