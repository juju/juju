// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

// ErrNoOperation is used to indicate that there are no
// currently pending operations to run.
var ErrNoOperation = errors.New("no operations")

var ErrWaiting = errors.New("waiting")

// ErrTerminate is used when the unit has been marked
// as dead and so there will never be any more
// operations to run for that unit.
var ErrTerminate = errors.New("terminate resolver")

// Resolver instances use local (as is) and remote (to be) state
// to provide operations to run in order to progress towards
// the desired state.
type Resolver interface {
	// NextOp returns the next operation to run to reconcile
	// the local state with the remote, desired state. This
	// method must return ErrNoOperation if there are no
	// operations to perform.
	//
	// By returning ErrTerminate, the resolver indicates that
	// it will never have any more operations to perform,
	// and the caller can cease calling.
	NextOp(operation.State, remotestate.Snapshot) (operation.Operation, error)
}
