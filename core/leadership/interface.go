// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package leadership holds code pertaining to application leadership in juju. It's
expected to grow as we're able to extract (e.g.) the Ticket and Tracker
interfaces from worker/leadership; and quite possible the implementations
themselves; but that'll have to wait until it can all be expressed without
reference to non-core code.
*/
package leadership

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
)

// TODO (manadart 2018-10-05) Add interfaces to the end of this line,
// separated by commas, as they become required for mocking in tests.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/core/leadership Pinner

// ErrClaimDenied is the error which will be returned when a
// leadership claim has been denied.
var ErrClaimDenied = errors.New("leadership claim denied")

// ErrBlockCancelled is returned from BlockUntilLeadershipReleased
// if the client cancels the request by closing the cancel channel.
var ErrBlockCancelled = errors.New("waiting for leadership cancelled by client")

// NewNotLeaderError returns an error indicating that this unit is not
// the leader of that application.
func NewNotLeaderError(unit, application string) error {
	return &notLeaderError{unit: unit, application: application}
}

type notLeaderError struct {
	unit        string
	application string
}

// Error is part of error.
func (e notLeaderError) Error() string {
	return fmt.Sprintf("%q is not leader of %q", e.unit, e.application)
}

// IsNotLeaderError returns whether this error represents a token
// check that failed because the unit in question wasn't the leader.
func IsNotLeaderError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := errors.Cause(err).(*notLeaderError)
	return ok
}

// Claimer exposes leadership acquisition capabilities.
type Claimer interface {

	// ClaimLeadership claims leadership of the named application on behalf of the
	// named unit. If no error is returned, leadership will be guaranteed for
	// at least the supplied duration from the point when the call was made.
	ClaimLeadership(applicationId, unitId string, duration time.Duration) error

	// BlockUntilLeadershipReleased blocks until the named application is known
	// to have no leader, in which case it returns no error; or until the
	// manager is stopped, in which case it will fail.
	BlockUntilLeadershipReleased(applicationId string, cancel <-chan struct{}) (err error)
}

// Pinner describes methods used to manage suspension of application leadership
// expiry. All methods should be idempotent.
type Pinner interface {

	// PinLeadership ensures that the leadership of the input application will
	// not expire. The input entity records the party responsible for the
	// pinning operation.
	PinLeadership(applicationId string, entity string) error

	// UnpinLeadership reverses a PinLeadership operation for the same
	// application and entity. Normal expiry behaviour is restored when no
	// entities remain with pins for the application.
	UnpinLeadership(applicationId string, entity string) error

	// PinnedLeadership returns a map keyed on pinned application names,
	// with entities that require the application's pinned behaviour.
	PinnedLeadership() map[string][]string
}

// Token represents a unit's leadership of its application.
type Token interface {

	// Check returns an error if the condition it embodies no longer
	// holds.  If you pass a non-nil trapdoorKey value into Check, it
	// must be a pointer to data of the correct type, into which the
	// token's content will be copied.
	//
	// The "correct type" is implementation-specific, and no implementation
	// is obliged to accept any non-nil parameter; but methods that return
	// Tokens should explain whether, and how, they will expose their content.
	//
	// In practice, most Token implementations will likely expect *[]txn.Op,
	// so that they can be used to gate mgo/txn-based state changes.
	//
	// If a non-nil trapdoorKey value is passed, attempt should be how
	// many times this check has been tried previously (for example,
	// in a buildTxn function it'll be the transaction attempt). This
	// enables the token to generate different ops for the second
	// attempt (the raft lease implementation uses this).
	Check(attempt int, trapdoorKey interface{}) error
}

// Checker exposes leadership testing capabilities.
type Checker interface {

	// LeadershipCheck returns a Token representing the supplied unit's
	// application leadership. The existence of the token does not imply
	// its accuracy; you need to Check() it.
	//
	// This method returns a token that accepts a *[]txn.Op, into which
	// it will (on success) copy mgo/txn operations that can be used to
	// verify the unit's continued leadership as part of another txn.
	LeadershipCheck(applicationId, unitId string) Token
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

	// ApplicationName returns the name of the application for which leadership claims
	// are made.
	ApplicationName() string

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

// TrackerWorker represents a leadership tracker worker.
type TrackerWorker interface {
	worker.Worker
	Tracker
}

// Reader describes the capability to read the current state of leadership.
type Reader interface {

	// Leaders returns all application leaders in the current model.
	// TODO (manadart 2019-02-27): The return in this signature includes error
	// in order to support state.ApplicationLeaders for legacy leases.
	// When legacy leases are removed, so can the error return.
	Leaders() (map[string]string, error)
}
