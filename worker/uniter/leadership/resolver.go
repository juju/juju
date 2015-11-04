// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

var logger = loggo.GetLogger("juju.worker.uniter.leadership")

type leadershipResolver struct {
}

// NewResolver returns a new leadership resolver.
func NewResolver() resolver.Resolver {
	return &leadershipResolver{}
}

// NextOp is defined on the Resolver interface.
func (l *leadershipResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {

	// TODO(wallyworld) - maybe this can occur before install
	if !localState.Installed {
		return nil, resolver.ErrNoOperation
	}

	// Check for any leadership change, and enact it if possible.
	logger.Tracef("checking leadership status")

	// If we've already accepted leadership, we don't need to do it again.
	canAcceptLeader := !localState.Leader
	if remoteState.Life == params.Dying {
		canAcceptLeader = false
	} else {
		// If we're in an unexpected mode (eg pending hook) we shouldn't try either.
		if localState.Kind != operation.Continue {
			canAcceptLeader = false
		}
	}

	switch {
	case remoteState.Leader && canAcceptLeader:
		return opFactory.NewAcceptLeadership()

	// If we're the leader but should not be any longer, or
	// if the unit is dying, we should resign leadership.
	case localState.Leader && (!remoteState.Leader || remoteState.Life == params.Dying):
		return opFactory.NewResignLeadership()
	}

	if localState.Kind == operation.Continue {
		// We want to run the leader settings hook if we're
		// not the leader and the settings have changed.
		if !localState.Leader && localState.LeaderSettingsVersion != remoteState.LeaderSettingsVersion {
			return opFactory.NewRunHook(hook.Info{Kind: hook.LeaderSettingsChanged})
		}
	}

	logger.Tracef("leadership status is up-to-date")
	return nil, resolver.ErrNoOperation
}
