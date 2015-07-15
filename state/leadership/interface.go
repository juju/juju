// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
)

// Token exposes mgo/txn operations that can be added to a transaction in order
// to abort it if the check that generated the Token would no longer pass.
type Token interface {

	// AssertOps returns mgo/txn operations that will abort if some fact no
	// longer holds true.
	AssertOps() []txn.Op
}

// Manager allows units to claim service leadership; to verify a unit's continued
// leadership of a service; and to wait until a service has no leader.
type Manager interface {

	// ClaimLeadership claims leadership of the named service on behalf of the
	// named unit. If no error is returned, leadership will be guaranteed for
	// at least the supplied duration from the point when the call was made.
	ClaimLeadership(serviceName, unitName string, duration time.Duration) error

	// CheckLeadership verifies that the named unit is leader of the named
	// service, and returns a Token attesting to that fact for use building
	// mgo/txn transactions that depend upon it.
	CheckLeadership(serviceName, unitName string) (Token, error)

	// BlockUntilLeadershipReleased blocks until the named service is known
	// to have no leader, in which case it returns no error; or until the
	// manager is stopped, in which case it will fail.
	BlockUntilLeadershipReleased(serviceName string) error
}

// ManagerWorker implements Manager and worker.Worker. We don't import worker
// because it pulls in a lot of dependencies and causes import cycles when you
// try to use leadership in state.
type ManagerWorker interface {
	Manager
	Kill()
	Wait() error
}

// errStopped is returned to clients when an operation cannot complete because
// the manager has started (and possibly finished) shutdown.
var errStopped = errors.New("leadership manager stopped")
