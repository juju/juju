// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package fortress implements a convenient metaphor for an RWLock (originally
designed to coordinate access to a charm directory among the uniter (which
sometimes deploys new charms) and the metrics collector (which needs to run
hooks on its own schedule; and can safely run hooks in parallel with other
hooks, but not while the charm directory is changing underneath it)).
*/
package fortress

import (
	"github.com/juju/errors"
)

// Guard manages Guest Visits to a fortress.
type Guard interface {

	// Unlock unblocks all Guest.Visit calls.
	Unlock() error

	// Lockdown blocks new Guest.Visit calls, and waits for existing calls to
	// complete. It will return ErrAborted if the supplied Abort is closed
	// before lockdown is complete.
	Lockdown(Abort) error
}

// Guest allows clients to Visit a fortress when it's unlocked.
type Guest interface {

	// Visit waits until the fortress is unlocked, then runs the supplied func.
	// It will return ErrAborted if the supplied Abort is closed before the
	// Visit is started.
	Visit(Visit, Abort) error
}

// Visit is an operation that can be performed by a Guest.
type Visit func() error

// Abort is a channel that can be closed to abort a blocking operation.
type Abort <-chan struct{}

// ErrAborted is used to signal clean termination of a blocking operation.
var ErrAborted = errors.New("fortress operation aborted")
