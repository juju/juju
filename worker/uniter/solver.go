package uniter

import (
	"fmt"

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
	opFactory      operation.Factory
	setAgentStatus func(params.Status, string, map[string]interface{}) error
	clearResolved  func() error

	charmURL      *charm.URL
	configVersion int

	leadershipSolver solver.Solver
	storageSolver    solver.Solver
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

	op, err := s.leadershipSolver.NextOp(opState, remoteState)
	if err != solver.ErrNoOperation {
		return op, err
	}

	op, err = s.storageSolver.NextOp(opState, remoteState)
	if err != solver.ErrNoOperation {
		return op, err
	}

	// Now that storage hooks have run at least once, before anything else,
	// we need to run the install hook.
	if !opState.Installed {
		if opState.Kind == operation.RunHook || opState.Kind == operation.Continue {
			opState.Hook = &hook.Info{Kind: hooks.Install}
			logger.Infof("found queued %q hook", opState.Hook.Kind)
			return s.opFactory.NewRunHook(*opState.Hook)
		} else {
			return nil, solver.ErrNoOperation
		}
	}

	switch opState.Kind {
	case operation.RunHook:
		switch opState.Step {
		case operation.Pending:
			logger.Infof("awaiting error resolution for %q hook", opState.Hook.Kind)
			break
		case operation.Queued:
			if !opState.Installed {
				opState.Hook = &hook.Info{Kind: hooks.Install}
			}
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

	// If we were running a hook, but failed to commit,
	// then we must wait for the error to be resolved.
	hookError := opState.Kind == operation.RunHook && opState.Step == operation.Pending

	if hookError {
		// TODO(axw) this feels out of place? do this in runHookOp?
		hookInfo := *opState.Hook
		hookName := string(hookInfo.Kind)
		statusData := map[string]interface{}{}
		/*
			if hookInfo.Kind.IsRelation() {
				statusData["relation-id"] = hookInfo.RelationId
				if hookInfo.RemoteUnit != "" {
					statusData["remote-unit"] = hookInfo.RemoteUnit
				}
				relationName, err := u.relations.Name(hookInfo.RelationId)
				if err != nil {
					return nil, errors.Trace(err)
				}
				hookName = fmt.Sprintf("%s-%s", relationName, hookInfo.Kind)
			}
		*/
		statusData["hook"] = hookName
		statusMessage := fmt.Sprintf("hook failed: %q", hookName)
		if err := s.setAgentStatus(
			params.StatusError, statusMessage, statusData,
		); err != nil {
			return nil, errors.Trace(err)
		}
	}

	if *s.charmURL != *remoteState.CharmURL {
		if !hookError || remoteState.ForceCharmUpgrade {
			logger.Debugf("upgrade from %v to %v", s.charmURL, remoteState.CharmURL)
			return s.opFactory.NewUpgrade(remoteState.CharmURL)
		}
	}

	if hookError {
		if remoteState.ResolvedMode != params.ResolvedNone {
			hookInfo := *opState.Hook
			switch remoteState.ResolvedMode {
			case params.ResolvedRetryHooks:
				if err := s.clearResolved(); err != nil {
					return nil, errors.Trace(err)
				}
				return s.opFactory.NewRunHook(hookInfo)
			case params.ResolvedNoHooks:
				if err := s.clearResolved(); err != nil {
					return nil, errors.Trace(err)
				}
				return s.opFactory.NewSkipHook(hookInfo)
			default:
				return nil, errors.Errorf(
					"unknown resolved mode %q", remoteState.ResolvedMode,
				)
			}
		}
		return nil, solver.ErrNoOperation
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
