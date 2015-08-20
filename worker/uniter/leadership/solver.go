// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	workerleadership "github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/solver"
)

var logger = loggo.GetLogger("juju.worker.uniter.leadership")

type leadershipSolver struct {
	opFactory operation.Factory
	tracker   workerleadership.Tracker

	ranLeaderSettingsChanged bool
}

// NewSolver returns a new leadership solver.
func NewSolver(opFactory operation.Factory, tracker workerleadership.Tracker) solver.Solver {
	return &leadershipSolver{
		opFactory: opFactory,
		tracker:   tracker,
	}
}

// NextOp is defined on the Solver interface.
func (l *leadershipSolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	// TODO(wallyworld) - maybe this can occur before install
	if !opState.Installed {
		return nil, solver.ErrNoOperation
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

	// NOTE: the Wait() looks scary, but a ClaimLeadership ticket should always
	// complete quickly; worst-case is API latency time, but it's designed that
	// it should be vanishingly rare to hit that code path.
	isLeader := l.tracker.ClaimLeader().Wait()
	switch {
	case isLeader && canAcceptLeader:
		return l.opFactory.NewAcceptLeadership()

	// If we're the leader but should not be any longer, or
	// if the unit is dying, we should resign leadership.
	case opState.Leader && (!isLeader || remoteState.Life == params.Dying):
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
	case operation.Continue:
		if opState.Started && !opState.Leader && !l.ranLeaderSettingsChanged {
			op, err := l.opFactory.NewRunHook(hook.Info{Kind: hook.LeaderSettingsChanged})
			if err != nil {
				return nil, err
			}
			return leadersettingsChangedWrapper{
				op, &l.ranLeaderSettingsChanged,
			}, nil
		}
	}

	logger.Infof("leadership status is up-to-date")
	return nil, solver.ErrNoOperation
}

type leadersettingsChangedWrapper struct {
	operation.Operation
	ranHook *bool
}

func (op leadersettingsChangedWrapper) Commit(state operation.State) (*operation.State, error) {
	st, err := op.Operation.Commit(state)
	if err != nil {
		return nil, err
	}
	*op.ranHook = true
	return st, nil
}
