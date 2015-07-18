// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
)

// Client manipulates leases backed by MongoDB. Client implementations are not
// expected to be goroutine-safe.
type Client interface {

	// ClaimLease records the supplied holder's claim to the supplied lease. If
	// it succeeds, the claim is guaranteed until at least the supplied duration
	// after the call to ClaimLease was initiated. If it returns ErrInvalid,
	// check Leases() for updated state.
	ClaimLease(lease string, request Request) error

	// ExtendLease records the supplied holder's continued claim to the supplied
	// lease, if necessary. If it succeeds, the claim is guaranteed until at
	// least the supplied duration after the call to ExtendLease was initiated.
	// If it returns ErrInvalid, check Leases() for updated state.
	ExtendLease(lease string, request Request) error

	// ExpireLease records the vacation of the supplied lease. It will fail if
	// we cannot verify that the lease's writer considers the expiry time to
	// have passed. If it returns ErrInvalid, check Leases() for updated state.
	ExpireLease(lease string) error

	// Leases returns a recent snapshot of lease state. Expiry times are
	// expressed according to the Clock the client was configured with.
	Leases() map[string]Info

	// Refresh reads all lease state from the database.
	Refresh() error
}

// Info holds information about a lease. It's MongoDB-specific, because it
// includes information that can be used with the mgo/txn package to gate
// transaction operations on lease state.
type Info struct {

	// Holder is the name of the current leaseholder.
	Holder string

	// Expiry is the latest time at which it's possible the lease might still
	// be valid. Attempting to expire the lease before this time will fail.
	Expiry time.Time

	// AssertOp, if included in a mgo/txn transaction, will gate the transaction
	// on the lease remaining held by Holder. If we didn't need this, we could
	// easily implement Clients backed by other substrates.
	AssertOp txn.Op
}

// Request describes a lease request.
type Request struct {

	// Holder identifies the lease holder.
	Holder string

	// Duration specifies the time for which the lease is required.
	Duration time.Duration
}

// Validate returns an error if any fields are invalid or inconsistent.
func (request Request) Validate() error {
	if err := validateString(request.Holder); err != nil {
		return errors.Annotatef(err, "invalid holder")
	}
	if request.Duration <= 0 {
		return errors.Errorf("invalid duration")
	}
	return nil
}

// ErrInvalid indicates that a client operation failed because latest state
// indicates that it's a logical impossibility. It's a short-range signal to
// calling code only; that code should never pass it on, but should inspect
// the Client's updated Leases() and either attempt a new operation or return
// a new error at a suitable level of abstraction.
var ErrInvalid = errors.New("invalid lease operation")
