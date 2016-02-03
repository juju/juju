// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package leadership holds code pertaining to service leadership in juju. It's
expected to grow as we're able to extract (e.g.) the Ticket and Tracker
interfaces from worker/leadership; and quite possible the implementations
themselves; but that'll have to wait until it can all be expressed without
reference to non-core code.
*/
package leadership

import (
	"time"

	"github.com/juju/errors"
)

// ErrClaimDenied is the error which will be returned when a
// leadership claim has been denied.
var ErrClaimDenied = errors.New("leadership claim denied")

// Claimer exposes leadership acquisition capabilities.
type Claimer interface {

	// ClaimLeadership claims leadership of the named service on behalf of the
	// named unit. If no error is returned, leadership will be guaranteed for
	// at least the supplied duration from the point when the call was made.
	ClaimLeadership(serviceId, unitId string, duration time.Duration) error

	// BlockUntilLeadershipReleased blocks until the named service is known
	// to have no leader, in which case it returns no error; or until the
	// manager is stopped, in which case it will fail.
	BlockUntilLeadershipReleased(serviceId string) (err error)
}

// Token represents a unit's leadership of its service.
type Token interface {

	// Check returns an error if the condition it embodies no longer holds.
	// If you pass a non-nil value into Check, it must be a pointer to data
	// of the correct type, into which the token's content will be copied.
	//
	// The "correct type" is implementation-specific, and no implementation
	// is obliged to accept any non-nil parameter; but methods that return
	// Tokens should explain whether, and how, they will expose their content.
	//
	// In practice, most Token implementations will likely expect *[]txn.Op,
	// so that they can be used to gate mgo/txn-based state changes.
	Check(interface{}) error
}

// Checker exposes leadership testing capabilities.
type Checker interface {

	// LeadershipCheck returns a Token representing the supplied unit's
	// service leadership. The existence of the token does not imply
	// its accuracy; you need to Check() it.
	//
	// This method returns a token that accepts a *[]txn.Op, into which
	// it will (on success) copy mgo/txn operations that can be used to
	// verify the unit's continued leadership as part of another txn.
	LeadershipCheck(serviceName, unitName string) Token
}

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
