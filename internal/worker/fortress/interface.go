// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress

import (
	"github.com/juju/errors"
)

// Guard manages Guest access to a fortress.
type Guard interface {

	// Unlock unblocks all Guest.Visit calls.
	Unlock() error

	// Lockdown blocks new Guest.Visit calls, and waits for existing calls to
	// complete; it will return ErrAborted if the supplied Abort is closed
	// before lockdown is complete. In this situation, the fortress will
	// remain closed to new visits, but may still be executing pre-existing
	// ones; you need to wait for a Lockdown to complete successfully before
	// you can infer exclusive access.
	Lockdown(Abort) error
}

// Guest allows clients to Visit a fortress when it's unlocked; that is, to
// get non-exclusive access to whatever resource is being protected for the
// duration of the supplied Visit func.
type Guest interface {

	// Visit waits until the fortress is unlocked, then runs the supplied
	// Visit func. It will return ErrAborted if the supplied Abort is closed
	// before the Visit is started.
	Visit(Visit, Abort) error
}

// Visit is an operation that can be performed by a Guest.
type Visit func() error

// Abort is a channel that can be closed to abort a blocking operation.
type Abort <-chan struct{}

// ErrAborted is used to confirm clean termination of a blocking operation.
var ErrAborted = errors.ConstError("fortress operation aborted")

// ErrShutdown is used to report that the fortress worker is shutting down.
var ErrShutdown = errors.ConstError("fortress worker shutting down")

// IsFortressError returns true if the error provided is fortress related.
func IsFortressError(err error) bool {
	switch errors.Cause(err) {
	case ErrAborted, ErrShutdown:
		return true
	default:
		return false
	}
}
