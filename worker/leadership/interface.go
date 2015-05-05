// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/juju/worker"
)

// Ticket is used to communicate leadership status to Tracker clients.
type Ticket interface {

	// Wait returns true if its Tracker is prepared to guarantee leadership
	// for some period from the ticket request. The guaranteed duration depends
	// upon the Tracker.
	Wait() bool

	// Ready returns a channel that will be closed when a result is available
	// to Wait(), and is helpful for clients that want to select rather than
	// block on long-waiting tickets.
	Ready() <-chan struct{}
}

// Tracker allows clients to discover current leadership status by attempting to
// claim it for themselves.
type Tracker interface {

	// ServiceName returns the name of the service for which leadership claims
	// are made.
	ServiceName() string

	// ClaimDuration returns the duration for which a Ticket's true Wait result
	// is guaranteed valid.
	ClaimDuration() time.Duration

	// ClaimLeader will return a Ticket which, when Wait()ed for, will return
	// true if leadership is guaranteed for at least the tracker's duration from
	// the time the ticket was issued. Leadership claims should be resolved
	// relatively quickly.
	ClaimLeader() Ticket

	// WaitLeader will return a Ticket which, when Wait()ed for, will block
	// until the tracker attains leadership.
	WaitLeader() Ticket

	// WaitMinion will return a Ticket which, when Wait()ed for, will block
	// until the tracker's future leadership can no longer be guaranteed.
	WaitMinion() Ticket
}

// TrackerWorker embeds the Tracker and worker.Worker interfaces.
type TrackerWorker interface {
	worker.Worker
	Tracker
}
