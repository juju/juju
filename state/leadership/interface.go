// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"

	"github.com/juju/juju/leadership"
)

// Secretary is reponsible for validating the sanity of lease and holder names
// before bothering the manager with them.
type Secretary interface {

	// CheckLease returns an error if the supplied lease name is not valid.
	CheckLease(name string) error

	// CheckHolder returns an error if the supplied holder name is not valid.
	CheckHolder(name string) error
}

// ManagerWorker implements leadership functions, and worker.Worker. We don't
// import worker because it pulls in a lot of dependencies and causes import
// cycles when you try to use leadership in state. We should break this cycle
// elsewhere if we can.
type ManagerWorker interface {
	leadership.Checker
	leadership.Claimer
	Kill()
	Wait() error
}

// errStopped is returned to clients when an operation cannot complete because
// the manager has started (and possibly finished) shutdown.
var errStopped = errors.New("leadership manager stopped")
