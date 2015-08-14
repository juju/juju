// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/lease"
)

// ErrClaimDenied is the error which will be returned when a
// leadership claim has been denied.
var ErrClaimDenied = errors.New("leadership claim denied")

type LeadershipManager interface {
	// ClaimLeadership claims a leadership for the given serviceId and
	// unitId. If successful, the leadership will persist for the supplied
	// duration (or until released).
	ClaimLeadership(serviceId, unitId string, duration time.Duration) error

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

	// RetrieveLease retrieves the current lease token for a given
	// namespace. This is not intended to be exposed to clients, and is
	// only available within a server-process.
	RetrieveLease(namespace string) (lease.Token, error)

	// LeaseReleasedNotifier returns a channel a caller can block on to be
	// notified of when a lease is released for namespace. The caller must
	// use a comma-ok to decide whether or not the channel was notified or
	// was closed: if notified, then the lease has been released; if closed,
	// then the lease manager has been disrupted before it could notify
	// the caller.
	LeaseReleasedNotifier(namespace string) (notifier <-chan struct{}, err error)
}
