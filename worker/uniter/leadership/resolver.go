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
	opFactory       operation.Factory
	settingsVersion int
}

// NewResolver returns a new leadership resolver.
func NewResolver(opFactory operation.Factory) resolver.Resolver {
	return &leadershipResolver{opFactory: opFactory}
}

// NextOp is defined on the Resolver interface.
func (l *leadershipResolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	// TODO(wallyworld) - maybe this can occur before install
	if !opState.Installed {
		return nil, resolver.ErrNoOperation
	}

	// Check for any leadership change, and enact it if possible.
	logger.Infof("checking leadership status")

	// If we've already accepted leadership, we don't need to do it again.
	canAcceptLeader := !opState.Leader
	if remoteState.Life == params.Dying {
		canAcceptLeader = false
	} else {
		// If we're in an unexpected mode (eg pending hook) we shouldn't try either.
		if opState.Kind != operation.Continue {
			canAcceptLeader = false
		}
	}

	switch {
	case remoteState.Leader && canAcceptLeader:
		return l.opFactory.NewAcceptLeadership()

	// If we're the leader but should not be any longer, or
	// if the unit is dying, we should resign leadership.
	case opState.Leader && (!remoteState.Leader || remoteState.Life == params.Dying):
		return l.opFactory.NewResignLeadership()
	}

	switch opState.Kind {
	case operation.RunHook:
		switch opState.Step {
		case operation.Queued:
			if opState.Hook.Kind == hook.LeaderElected {
				logger.Infof("found queued %q hook", opState.Hook.Kind)
				return l.opFactory.NewRunHook(*opState.Hook)
			}
		}
	}

	// We want to run the leader settings hook if we're not the leader
	// and the settings have changed.
	if !opState.Leader && l.settingsVersion != remoteState.LeaderSettingsVersion {
		op, err := l.opFactory.NewRunHook(hook.Info{Kind: hook.LeaderSettingsChanged})
		if err != nil {
			return nil, err
		}
		return leadersettingsChangedWrapper{
			op, &l.settingsVersion, remoteState.LeaderSettingsVersion,
		}, nil
	}

	logger.Infof("leadership status is up-to-date")
	return nil, resolver.ErrNoOperation
}

type leadersettingsChangedWrapper struct {
	operation.Operation
	oldVersion *int
	newVersion int
}

func (op leadersettingsChangedWrapper) Commit(state operation.State) (*operation.State, error) {
	st, err := op.Operation.Commit(state)
	if err != nil {
		return nil, err
	}
	*op.oldVersion = op.newVersion
	return st, nil
}
