// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
<<<<<<< HEAD
Package fortress implements a convenient metaphor for an RWLock.

A "fortress" is constructed via a manifold's Start func, and accessed via its
Output func as either a Guard or a Guest. To begin with, it's considered to be
locked, and inaccessible to Guests; when the Guard Unlocks it, the Guests can
Visit it until the Guard calls Lockdown. At that point, new Visits are blocked,
and existing Visits are allowed to complete; the Lockdown returns once all
Guests' Visits have completed.

The original motivating use case was for a component to mediate charm directory
access between the uniter and the metrics collector. The metrics collector must
be free to run its own independent hooks while the uniter is active; but metrics
hooks and charm upgrades cannot be allowed to tread on one another's toes.
=======
Package fortress implements a convenient metaphor for an RWLock (originally
designed to coordinate access to a charm directory among the uniter (which
sometimes deploys new charms) and the metrics collector (which needs to run
hooks on its own schedule; and can safely run hooks in parallel with other
hooks, but not while the charm directory is changing underneath it)).
>>>>>>> add worker/fortress (intended to replace worker/charmdir..?)
*/
package fortress

import (
	"github.com/juju/errors"
)

<<<<<<< HEAD
// Guard manages Guest access to a fortress.
=======
// Guard manages Guest Visits to a fortress.
>>>>>>> add worker/fortress (intended to replace worker/charmdir..?)
type Guard interface {

	// Unlock unblocks all Guest.Visit calls.
	Unlock() error

	// Lockdown blocks new Guest.Visit calls, and waits for existing calls to
	// complete. It will return ErrAborted if the supplied Abort is closed
	// before lockdown is complete.
	Lockdown(Abort) error
}

<<<<<<< HEAD
// Guest allows clients to Visit a fortress when it's unlocked; that is, to
// get non-exclusive access to whatever resource is being protected for the
// duration of the supplied Visit func.
type Guest interface {

	// Visit waits until the fortress is unlocked, then runs the supplied
	// Visit func. It will return ErrAborted if the supplied Abort is closed
	// before the Visit is started.
=======
// Guest allows clients to Visit a fortress when it's unlocked.
type Guest interface {

	// Visit waits until the fortress is unlocked, then runs the supplied func.
	// It will return ErrAborted if the supplied Abort is closed before the
	// Visit is started.
>>>>>>> add worker/fortress (intended to replace worker/charmdir..?)
	Visit(Visit, Abort) error
}

// Visit is an operation that can be performed by a Guest.
type Visit func() error

// Abort is a channel that can be closed to abort a blocking operation.
type Abort <-chan struct{}

<<<<<<< HEAD
// ErrAborted is used to confirm clean termination of a blocking operation.
=======
// ErrAborted is used to signal clean termination of a blocking operation.
>>>>>>> add worker/fortress (intended to replace worker/charmdir..?)
var ErrAborted = errors.New("fortress operation aborted")
