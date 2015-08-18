package uniter

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/solver"
)

type leadershipSolver struct {
	opFactory operation.Factory
	tracker   leadership.Tracker
}

func (l *leadershipSolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

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
	case opState.Leader && !isLeader:
		return l.opFactory.NewResignLeadership()
	}
	logger.Infof("leadership status is up-to-date")

	// TODO(axw) ModeAbide bits

	return nil, solver.ErrNoOperation
}
