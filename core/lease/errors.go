// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import "github.com/juju/juju/internal/errors"

const (
	// ErrClaimDenied indicates that a Claimer.Claim() has been denied.
	ErrClaimDenied = errors.ConstError("lease claim denied")

	// ErrNotHeld indicates that some holder does not hold some lease.
	ErrNotHeld = errors.ConstError("lease not held")

	// ErrWaitCancelled is returned by Claimer.WaitUntilExpired if the
	// cancel channel is closed.
	ErrWaitCancelled = errors.ConstError("waiting for lease cancelled by client")

	// ErrInvalid indicates that a Store operation failed because latest state
	// indicates that it's a logical impossibility. It's a short-range signal to
	// calling code only; that code should never pass it on, but should inspect
	// the Store's updated Leases() and either attempt a new operation or return
	// a new error at a suitable level of abstraction.
	ErrInvalid = errors.ConstError("invalid lease operation")

	// ErrHeld indicates that a claim operation was impossible to fulfill
	// because the lease has been claimed on behalf of another entity.
	// This operation should not be retried.
	ErrHeld = errors.ConstError("lease already held")

	// ErrTimeout indicates that a Store operation failed because it
	// couldn't update the underlying lease information. This is probably
	// a transient error due to changes in the cluster, and indicates that
	// the operation should be retried.
	ErrTimeout = errors.ConstError("lease operation timed out")

	// ErrAborted indicates that the stop channel returned before the operation
	// succeeded or failed.
	ErrAborted = errors.ConstError("lease operation aborted")
)

// IsInvalid returns whether the specified error represents ErrInvalid
// (even if it's wrapped).
func IsInvalid(err error) bool {
	return errors.Is(err, ErrInvalid)
}

// IsHeld returns whether the specified error represents ErrHeld
// (even if it's wrapped).
func IsHeld(err error) bool {
	return errors.Is(err, ErrHeld)
}

// IsTimeout returns whether the specified error represents ErrTimeout
// (even if it's wrapped).
func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout)
}

// IsAborted returns whether the specified error represents ErrAborted
// (even if it's wrapped).
func IsAborted(err error) bool {
	return errors.Is(err, ErrAborted)
}

// IsNotHeld returns whether the specified error represents ErrNotHeld
// (even if it's wrapped).
func IsNotHeld(err error) bool {
	return errors.Is(err, ErrNotHeld)
}
