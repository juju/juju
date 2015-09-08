// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

type ResolverFunc func(
	LocalState,
	remotestate.Snapshot,
	operation.Factory,
) (operation.Operation, error)

func (f ResolverFunc) NextOp(
	local LocalState,
	remote remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	return f(local, remote, opFactory)
}
