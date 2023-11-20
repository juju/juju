// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"context"

	"github.com/juju/charm/v11/hooks"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed into the NewResolver as needed.
type logger interface{}

var _ logger = struct{}{}

// Logger defines the logging methods used by the leadership package.
type Logger interface {
	Tracef(string, ...interface{})
}

type leadershipResolver struct {
	logger Logger
}

// NewResolver returns a new leadership resolver.
func NewResolver(logger Logger) resolver.Resolver {
	return &leadershipResolver{logger: logger}
}

// NextOp is defined on the Resolver interface.
func (l *leadershipResolver) NextOp(
	ctx context.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {

	// TODO(wallyworld) - maybe this can occur before install
	if !localState.Installed {
		return nil, resolver.ErrNoOperation
	}

	// Check for any leadership change, and enact it if possible.
	l.logger.Tracef("checking leadership status")

	// If we've already accepted leadership, we don't need to do it again.
	canAcceptLeader := !localState.Leader
	if remoteState.Life == life.Dying {
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
	case localState.Leader && (!remoteState.Leader || remoteState.Life == life.Dying):
		return opFactory.NewResignLeadership()
	}

	if localState.Kind == operation.Continue {
		// We want to run the leader settings hook if we're
		// not the leader and the settings have changed.
		// Note though that if we are dying, we may have already executed "resign leadership".
		// In this case, as far as the unit agent is concerned, we are not the leader any more
		// but we don't want to run the leader settings hook as the transition away from leadership
		// has only been recorded locally, and the Juju model still has us as a leader that is dying.
		if !localState.Leader && remoteState.Life != life.Dying && localState.LeaderSettingsVersion != remoteState.LeaderSettingsVersion {
			return opFactory.NewRunHook(hook.Info{Kind: hooks.LeaderSettingsChanged})
		}
	}

	l.logger.Tracef("leadership status is up-to-date")
	return nil, resolver.ErrNoOperation
}
