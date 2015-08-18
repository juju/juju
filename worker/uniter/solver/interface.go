// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package solver

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

var ErrNoOperation = errors.New("no operations")

var ErrTerminate = errors.New("terminate solver")

type Solver interface {
	// NextOp returns the next operation to run to reconcile
	// the local state with the remote, desired state. This
	// method must return ErrNoOperation if there are no
	// operations to perform.
	//
	// By returning ErrTerminate, the solver indicates that
	// it will never have any more operations to perform,
	// and the caller can cease calling.
	NextOp(operation.State, remotestate.Snapshot) (operation.Operation, error)
}
