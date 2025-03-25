// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"context"
	"strings"
	"time"

	"github.com/juju/juju/internal/errors"
)

// Store manipulates leases directly, and is most likely to be seen set on a
// worker/lease.ManagerConfig struct (and used by the Manager). Implementations
// of Store are not expected to be goroutine-safe.
type Store interface {
	// ClaimLease records the supplied holder's claim to the supplied lease. If
	// it succeeds, the claim is guaranteed until at least the supplied duration
	// after the call to ClaimLease was initiated. If it returns ErrInvalid,
	// check Leases() for updated state.
	ClaimLease(ctx context.Context, lease Key, request Request) error

	// ExtendLease records the supplied holder's continued claim to the supplied
	// lease, if necessary. If it succeeds, the claim is guaranteed until at
	// least the supplied duration after the call to ExtendLease was initiated.
	// If it returns ErrInvalid, check Leases() for updated state.
	ExtendLease(ctx context.Context, lease Key, request Request) error

	// RevokeLease records the vacation of the supplied lease. It will fail if
	// the lease is not held by the holder.
	RevokeLease(ctx context.Context, lease Key, holder string) error

	// Leases returns a recent snapshot of lease state. Expiry times are
	// expressed according to the Clock the store was configured with.
	// Supplying any lease keys will filter the return for those requested.
	Leases(ctx context.Context, keys ...Key) (map[Key]Info, error)

	// LeaseGroup returns a snapshot of all of the leases for a
	// particular namespace/model combination. This is useful for
	// reporting holder information for a model, and can often be
	// implemented more efficiently than getting all leases when there
	// are many models.
	LeaseGroup(ctx context.Context, namespace, modelUUID string) (map[Key]Info, error)

	// PinLease ensures that the current holder of the lease for the input key
	// will not lose the lease to expiry.
	// If there is no current holder of the lease, the next claimant will be
	// the recipient of the pin behaviour.
	// The input entity denotes the party responsible for the
	// pinning operation.
	PinLease(ctx context.Context, lease Key, entity string) error

	// UnpinLease reverses a Pin operation for the same key and entity.
	// Normal expiry behaviour is restored when no entities remain with
	// pins for the application.
	UnpinLease(ctx context.Context, lease Key, entity string) error

	// Pinned returns a snapshot of pinned leases.
	// The return consists of each pinned lease and the collection of entities
	// requiring its pinned behaviour.
	Pinned(ctx context.Context) (map[Key][]string, error)
}

// ExpiryStore defines a store for expiring leases.
type ExpiryStore interface {
	// ExpireLeases deletes all leases that have expired.
	ExpireLeases(ctx context.Context) error
}

// Key fully identifies a lease, including the namespace and
// model it belongs to.
type Key struct {
	Namespace string
	ModelUUID string
	Lease     string
}

// Info holds substrate-independent information about a lease.
type Info struct {
	// Holder is the name of the current leaseholder.
	Holder string

	// Expiry is the latest time at which it's possible the lease might still
	// be valid. Attempting to expire the lease before this time will fail.
	Expiry time.Time
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
		return errors.Errorf("invalid holder: %w", err)
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
