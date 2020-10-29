// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"strings"
	"time"

	"github.com/juju/errors"
)

// Store manipulates leases directly, and is most likely to be seen set on a
// worker/lease.ManagerConfig struct (and used by the Manager). Implementations
// of Store are not expected to be goroutine-safe.
type Store interface {

	// ClaimLease records the supplied holder's claim to the supplied lease. If
	// it succeeds, the claim is guaranteed until at least the supplied duration
	// after the call to ClaimLease was initiated. If it returns ErrInvalid,
	// check Leases() for updated state.
	ClaimLease(lease Key, request Request, stop <-chan struct{}) error

	// ExtendLease records the supplied holder's continued claim to the supplied
	// lease, if necessary. If it succeeds, the claim is guaranteed until at
	// least the supplied duration after the call to ExtendLease was initiated.
	// If it returns ErrInvalid, check Leases() for updated state.
	ExtendLease(lease Key, request Request, stop <-chan struct{}) error

	// RevokeLease records the vacation of the supplied lease. It will fail if
	// the lease is not held by the holder.
	RevokeLease(lease Key, holder string, stop <-chan struct{}) error

	// Leases returns a recent snapshot of lease state. Expiry times are
	// expressed according to the Clock the store was configured with.
	// Supplying any lease keys will filter the return for those requested.
	Leases(keys ...Key) map[Key]Info

	// LeaseGroup returns a snapshot of all of the leases for a
	// particular namespace/model combination. This is useful for
	// reporting holder information for a model, and can often be
	// implemented more efficiently than getting all leases when there
	// are many models.
	LeaseGroup(namespace, modelUUID string) map[Key]Info

	// Refresh reads all lease state from the database.
	Refresh() error

	// Pin ensures that the current holder of the lease for the input key will
	// not lose the lease to expiry.
	// If there is no current holder of the lease, the next claimant will be
	// the recipient of the pin behaviour.
	// The input entity denotes the party responsible for the
	// pinning operation.
	PinLease(lease Key, entity string, stop <-chan struct{}) error

	// Unpin reverses a Pin operation for the same key and entity.
	// Normal expiry behaviour is restored when no entities remain with
	// pins for the application.
	UnpinLease(lease Key, entity string, stop <-chan struct{}) error

	// Pinned returns a snapshot of pinned leases.
	// The return consists of each pinned lease and the collection of entities
	// requiring its pinned behaviour.
	Pinned() map[Key][]string
}

// Key fully identifies a lease, including the namespace and
// model it belongs to.
type Key struct {
	Namespace string
	ModelUUID string
	Lease     string
}

// Info holds substrate-independent information about a lease; and a substrate-
// specific trapdoor func.
type Info struct {

	// Holder is the name of the current leaseholder.
	Holder string

	// Expiry is the latest time at which it's possible the lease might still
	// be valid. Attempting to expire the lease before this time will fail.
	Expiry time.Time

	// Trapdoor exposes the originating Store's persistence substrate, if the
	// substrate exposes any such capability. It's useful specifically for
	// integrating mgo/txn-based components: which thus get a mechanism for
	// extracting assertion operations they can use to gate other substrate
	// changes on lease state.
	Trapdoor Trapdoor
}

// Trapdoor allows a store to use pre-agreed special knowledge to communicate
// with a Store substrate by passing a key with suitable properties.
type Trapdoor func(attempt int, key interface{}) error

// LockedTrapdoor is a Trapdoor suitable for use by substrates that don't want
// or need to expose their internals.
func LockedTrapdoor(attempt int, key interface{}) error {
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
// whitespace, or if it contains any character in `.#$`. Store implementations
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

var (
	// ErrInvalid indicates that a Store operation failed because latest state
	// indicates that it's a logical impossibility. It's a short-range signal to
	// calling code only; that code should never pass it on, but should inspect
	// the Store's updated Leases() and either attempt a new operation or return
	// a new error at a suitable level of abstraction.
	ErrInvalid = errors.New("invalid lease operation")

	// ErrHeld indicates that a claim operation was impossible to fulfill
	// because the lease is already held.
	// This operation should not be retried.
	ErrHeld = errors.New("lease already held")

	// ErrTimeout indicates that a Store operation failed because it
	// couldn't update the underlying lease information. This is probably
	// a transient error due to changes in the cluster, and indicates that
	// the operation should be retried.
	ErrTimeout = errors.New("lease operation timed out")

	// ErrAborted indicates that the stop channel returned before the operation
	// succeeded or failed.
	ErrAborted = errors.New("lease operation aborted")
)

// IsInvalid returns whether the specified error represents ErrInvalid
// (even if it's wrapped).
func IsInvalid(err error) bool {
	return errors.Cause(err) == ErrInvalid
}

// IsHeld returns whether the specified error represents ErrHeld
// (even if it's wrapped).
func IsHeld(err error) bool {
	return errors.Cause(err) == ErrHeld
}

// IsTimeout returns whether the specified error represents ErrTimeout
// (even if it's wrapped).
func IsTimeout(err error) bool {
	return errors.Cause(err) == ErrTimeout
}

// IsAborted returns whether the specified error represents ErrAborted
// (even if it's wrapped).
func IsAborted(err error) bool {
	return errors.Cause(err) == ErrAborted
}

// IsNotHeld returns whether the specified error represents ErrNotHeld
// (even if it's wrapped).
func IsNotHeld(err error) bool {
	return errors.Cause(err) == ErrNotHeld
}
