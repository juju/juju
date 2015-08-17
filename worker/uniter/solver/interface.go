// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package solver

import (
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

type Solver interface {
	// NextOp returns the next operation to run to reconcile
	// the local state with the remote, desired state. This
	// method may return nil to indicate that there are no
	// operations to perform.
	NextOp(remotestate.Snapshot) operation.Operation
}
