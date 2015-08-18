package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/solver"
	"gopkg.in/juju/charm.v5/hooks"
)

type uniterSolver struct {
	opFactory operation.Factory

	configVersion int

	leadershipSolver solver.Solver
	//storageSolver   solver.Solver
	//relationsSolver solver.Solver
}

func (s *uniterSolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {
	if opState.Kind == operation.Upgrade {
		logger.Infof("resuming charm upgrade")
		return s.opFactory.NewUpgrade(opState.CharmURL)
	}

	for _, s := range [...]solver.Solver{
		s.leadershipSolver,
		//s.storageSolver,
		//s.relationsSolver,
	} {
		op, err := s.NextOp(opState, remoteState)
		if err == solver.ErrNoOperation {
			continue
		}
		return op, err
	}

	switch opState.Kind {
	case operation.RunHook:
		switch opState.Step {
		case operation.Pending:
			// FIXME
			//logger.Infof("awaiting error resolution for %q hook", opState.Hook.Kind)
			//return ModeHookError, nil
			panic("TODO: handle error-resolution")
		case operation.Queued:
			logger.Infof("found queued %q hook", opState.Hook.Kind)
			return s.opFactory.NewRunHook(*opState.Hook)
		case operation.Done:
			logger.Infof("committing %q hook", opState.Hook.Kind)
			return s.opFactory.NewSkipHook(*opState.Hook)
		}

	case operation.Continue:
		if opState.Stopped {
			// The unit is stopping, so tell the caller to stop
			// calling us. The caller is then responsible for
			// terminating the agent.
			return nil, solver.ErrTerminate
		}
		logger.Infof("no operations in progress; waiting for changes")
		break

	default:
		return nil, errors.Errorf("unknown operation kind %v", opState.Kind)
	}

	return s.nextOp(opState, remoteState)
}

func (s *uniterSolver) nextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	// TODO(axw) agent status

	if s.configVersion != remoteState.ConfigVersion {
		op, err := s.opFactory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
		if err != nil {
			return nil, err
		}
		return updateVersionHookWrapper{
			op, &s.configVersion, remoteState.ConfigVersion,
		}, nil
	}

	return nil, solver.ErrNoOperation
}

type updateVersionHookWrapper struct {
	operation.Operation
	oldVersion *int
	newVersion int
}

func (op updateVersionHookWrapper) Commit(state operation.State) (*operation.State, error) {
	st, err := op.Operation.Commit(state)
	if err != nil {
		return nil, err
	}
	*op.oldVersion = op.newVersion
	return st, nil
}
