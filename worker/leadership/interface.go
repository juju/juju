// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/juju/worker"
)

// Ticket is used to communicate leadership status to Tracker clients.
type Ticket interface {

	// Wait returns true if its Tracker is prepared to guarantee leadership
	// for some period from the ticket request. The guaranteed duration depends
	// upon the Tracker.
	Wait() bool
}

// Tracker allows clients to discover current leadership status by attempting to
// claim it for themselves.
type Tracker interface {

	// ClaimLeader will return a Ticket which, when Wait()ed for, will return
	// true if leadership is guaranteed for at least the tracker's duration from
	// the time the ticket was issued.
	ClaimLeader() Ticket
}

// TrackerWorker embeds the Tracker and worker.Worker interfaces.
type TrackerWorker interface {
	worker.Worker
	Tracker
}
