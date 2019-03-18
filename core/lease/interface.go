// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
)

const (
	// ApplicationLeadershipNamespace is the namespace used to manage
	// leadership leases.
	ApplicationLeadershipNamespace = "application-leadership"

	// SingularControllerNamespace is the namespace used to manage
	// controller leases.
	SingularControllerNamespace = "singular-controller"
)

// ErrClaimDenied indicates that a Claimer.Claim() has been denied.
var ErrClaimDenied = errors.New("lease claim denied")

// ErrNotHeld indicates that some holder does not hold some lease.
var ErrNotHeld = errors.New("lease not held")

// ErrWaitCancelled is returned by Claimer.WaitUntilExpired if the
// cancel channel is closed.
var ErrWaitCancelled = errors.New("waiting for lease cancelled by client")

// Claimer exposes lease acquisition and expiry notification capabilities.
type Claimer interface {

	// Claim acquires or extends the named lease for the named holder. If it
	// succeeds, the holder is guaranteed to keep the lease until at least
	// duration after the *start* of the call. If it returns ErrClaimDenied,
	// the holder is guaranteed not to have the lease. If it returns any other
	// error, no reasonable inferences may be made.
	Claim(leaseName, holderName string, duration time.Duration) error

	// WaitUntilExpired returns nil when the named lease is no longer held. If it
	// returns any error, no reasonable inferences may be made. If the supplied
	// cancel channel is non-nil, it can be used to cancel the request; in this
	// case, the method will return ErrWaitCancelled.
	WaitUntilExpired(leaseName string, cancel <-chan struct{}) error
}

// Pinner describes methods used to manage suspension of lease expiry.
type Pinner interface {

	// Pin ensures that the current holder of input lease name will not lose
	// the lease to expiry.
	// If there is no current holder of the lease, the next claimant will be
	// the recipient of the pin behaviour.
	// The input entity denotes the party responsible for the
	// pinning operation.
	Pin(leaseName string, entity string) error

	// Unpin reverses a Pin operation for the same application and entity.
	// Normal expiry behaviour is restored when no entities remain with
	// pins for the application.
	Unpin(leaseName string, entity string) error

	// Pinned returns all names for pinned leases, with the entities requiring
	// their pinned behaviour.
	Pinned() map[string][]string
}

// Checker exposes facts about lease ownership.
type Checker interface {

	// Token returns a Token that can be interrogated at any time to discover
	// whether the supplied lease is currently held by the supplied holder.
	Token(leaseName, holderName string) Token
}

// Token represents a fact -- but not necessarily a *true* fact -- about some
// holder's ownership of some lease.
type Token interface {

	// Check returns ErrNotHeld if the lease it represents is not held
	// by the holder it represents. If attempt is 0, trapdoorKey is
	// nil, and Check returns nil, then the token continues to
	// represent a true fact.
	//
	// Attempt is the number of times this check has been tried (not
	// necessarily with this token instance). This enables an
	// implementation to vary how it deals with trapdoorKey across
	// attempts. For example, the raft implementation generates
	// different ops for the first attempt than for subsequent
	// attempts.
	//
	// If the token represents a true fact and trapdoorKey is *not*
	// nil, it will be passed through layers for the attention of the
	// underlying lease.Client implementation. If you need to do this,
	// consult the documentation for the particular Client you're
	// using to determine what key should be passed and what errors
	// that might induce.
	Check(attempt int, trapdoorKey interface{}) error
}

// Reader describes retrieval of all leases and holders
// for a known namespace and model.
type Reader interface {
	Leases() map[string]string
}

// Manager describes methods for acquiring objects that manipulate and query
// leases for different models.
type Manager interface {
	Checker(namespace string, modelUUID string) (Checker, error)
	Claimer(namespace string, modelUUID string) (Claimer, error)
	Pinner(namespace string, modelUUID string) (Pinner, error)
	Reader(namespace string, modelUUID string) (Reader, error)
}
