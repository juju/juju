// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"context"

	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

type ResolverFunc func(
	context.Context,
	LocalState,
	remotestate.Snapshot,
	operation.Factory,
) (operation.Operation, error)

func (f ResolverFunc) NextOp(
	ctx context.Context,
	local LocalState,
	remote remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	return f(ctx, local, remote, opFactory)
}
