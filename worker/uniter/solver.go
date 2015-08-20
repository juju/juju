package uniter

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/solver"
)

type uniterSolver struct {
	opFactory       operation.Factory
	clearResolved   func() error
	reportHookError func(hook.Info) error

	charmURL      *charm.URL
	configVersion int

	leadershipSolver solver.Solver
	relationsSolver  solver.Solver
	storageSolver    solver.Solver
}

func (s *uniterSolver) NextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	if opState.Kind == operation.Upgrade {
		logger.Infof("resuming charm upgrade")
		return s.opFactory.NewUpgrade(opState.CharmURL)
	}

	op, err := s.leadershipSolver.NextOp(opState, remoteState)
	if errors.Cause(err) != solver.ErrNoOperation {
		return op, err
	}

	op, err = s.storageSolver.NextOp(opState, remoteState)
	if errors.Cause(err) != solver.ErrNoOperation {
		return op, err
	}

	switch opState.Kind {
	case operation.RunHook:
		switch opState.Step {
		case operation.Pending:
			logger.Infof("awaiting error resolution for %q hook", opState.Hook.Kind)
			return s.nextOpHookError(opState, remoteState)

		case operation.Queued:
			logger.Infof("found queued %q hook", opState.Hook.Kind)
			return s.opFactory.NewRunHook(*opState.Hook)

		case operation.Done:
			logger.Infof("committing %q hook", opState.Hook.Kind)
			return s.opFactory.NewSkipHook(*opState.Hook)

		default:
			return nil, errors.Errorf("unknown operation step %v", opState.Step)
		}

	case operation.Continue:
		logger.Infof("no operations in progress; waiting for changes")
		return s.nextOp(opState, remoteState)

	default:
		return nil, errors.Errorf("unknown operation kind %v", opState.Kind)
	}
}

func (s *uniterSolver) nextOpHookError(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	// Report the hook error.
	if err := s.reportHookError(*opState.Hook); err != nil {
		return nil, errors.Trace(err)
	}

	if remoteState.ForceCharmUpgrade && *s.charmURL != *remoteState.CharmURL {
		logger.Debugf("upgrade from %v to %v", s.charmURL, remoteState.CharmURL)
		return s.opFactory.NewUpgrade(remoteState.CharmURL)
	}

	switch remoteState.ResolvedMode {
	case params.ResolvedNone:
		return nil, solver.ErrNoOperation
	case params.ResolvedRetryHooks:
		if err := s.clearResolved(); err != nil {
			return nil, errors.Trace(err)
		}
		return s.opFactory.NewRunHook(*opState.Hook)
	case params.ResolvedNoHooks:
		if err := s.clearResolved(); err != nil {
			return nil, errors.Trace(err)
		}
		return s.opFactory.NewSkipHook(*opState.Hook)
	default:
		return nil, errors.Errorf(
			"unknown resolved mode %q", remoteState.ResolvedMode,
		)
	}
}

func (s *uniterSolver) nextOp(
	opState operation.State,
	remoteState remotestate.Snapshot,
) (operation.Operation, error) {

	switch remoteState.Life {
	case params.Alive:
	case params.Dying:
		// Normally we handle relations last, but if we're dying we
		// must ensure that all relations are broken first.
		op, err := s.relationsSolver.NextOp(opState, remoteState)
		if errors.Cause(err) != solver.ErrNoOperation {
			return op, err
		}

		// We're not in a hook error and the unit is Dying,
		// so we should proceed to tear down.
		//
		// TODO(axw) u.unit.DestroyAllSubordinates()
		// TODO(axw) move logic for cascading destruction of
		//           subordinates, relation units and storage
		//           attachments into state, via cleanups.
		if opState.Started && !opState.Stopped {
			return s.opFactory.NewRunHook(hook.Info{Kind: hooks.Stop})
		}
		fallthrough

	case params.Dead:
		// The unit is dying/dead and stopped, so tell the uniter
		// to terminate.
		return nil, solver.ErrTerminate
	}

	// Now that storage hooks have run at least once, before anything else,
	// we need to run the install hook.
	if !opState.Installed {
		return s.opFactory.NewRunHook(hook.Info{Kind: hooks.Install})
	}

	if *s.charmURL != *remoteState.CharmURL {
		logger.Debugf("upgrade from %v to %v", s.charmURL, remoteState.CharmURL)
		return s.opFactory.NewUpgrade(remoteState.CharmURL)
	}

	if s.configVersion != remoteState.ConfigVersion {
		op, err := s.opFactory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
		if err != nil {
			return nil, err
		}
		return updateVersionHookWrapper{
			op, &s.configVersion, remoteState.ConfigVersion,
		}, nil
	}

	return s.relationsSolver.NextOp(opState, remoteState)
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
