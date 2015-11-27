// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"strings"
	"time"

	"github.com/juju/errors"
)

// Client manipulates leases directly, and is most likely to be seen set on a
// worker/lease.ManagerConfig struct (and used by the Manager). Implementations
// of Client are not expected to be goroutine-safe.
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

// Info holds substrate-independent information about a lease; and a substrate-
// specific trapdoor func.
type Info struct {

	// Holder is the name of the current leaseholder.
	Holder string

	// Expiry is the latest time at which it's possible the lease might still
	// be valid. Attempting to expire the lease before this time will fail.
	Expiry time.Time

	// Trapdoor exposes the originating Client's persistence substrate, if the
	// substrate exposes any such capability. It's useful specifically for
	// integrating mgo/txn-based components: which thus get a mechanism for
	// extracting assertion operations they can use to gate other substrate
	// changes on lease state.
	Trapdoor Trapdoor
}

// Trapdoor allows a client to use pre-agreed special knowledge to communicate
// with a Client substrate by passing a key with suitable properties.
type Trapdoor func(key interface{}) error

// LockedTrapdoor is a Trapdoor suitable for use by substrates that don't want
// or need to expose their internals.
func LockedTrapdoor(key interface{}) error {
	if key != nil {
		return errors.New("lease substrate not accessible")
	}
	return nil
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
	if err := ValidateString(request.Holder); err != nil {
		return errors.Annotatef(err, "invalid holder")
	}
	if request.Duration <= 0 {
		return errors.Errorf("invalid duration")
	}
	return nil
}

// ValidateString returns an error if the string is empty, or if it contains
// whitespace, or if it contains any character in `.#$`. Client implementations
// are expected to always reject invalid strings, and never to produce them.
func ValidateString(s string) error {
	if s == "" {
		return errors.New("string is empty")
	}
	if strings.ContainsAny(s, ".$# \t\r\n") {
		return errors.New("string contains forbidden characters")
	}
	return nil
}

// ErrInvalid indicates that a Client operation failed because latest state
// indicates that it's a logical impossibility. It's a short-range signal to
// calling code only; that code should never pass it on, but should inspect
// the Client's updated Leases() and either attempt a new operation or return
// a new error at a suitable level of abstraction.
var ErrInvalid = errors.New("invalid lease operation")
