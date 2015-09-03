// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type uniterResolver struct {
	clearResolved   func() error
	reportHookError func(hook.Info) error
	fixDeployer     func() error

	leadershipResolver resolver.Resolver
	actionsResolver    resolver.Resolver
	relationsResolver  resolver.Resolver
	storageResolver    resolver.Resolver
}

func (s *uniterResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {

	if remoteState.Life == params.Dead || localState.Stopped {
		return nil, resolver.ErrTerminate
	}

	if localState.Kind == operation.Upgrade {
		if localState.Conflicted {
			return s.nextOpConflicted(localState, remoteState, opFactory)
		}
		logger.Infof("resuming charm upgrade")
		return opFactory.NewUpgrade(localState.CharmURL)
	}

	if localState.Restart {
		// We've just run the upgrade op, which will change the
		// unit's charm URL. We need to restart the resolver
		// loop so that we start watching the correct events.
		return nil, resolver.ErrRestart
	}

	if localState.Kind == operation.Continue {
		if err := s.fixDeployer(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	op, err := s.leadershipResolver.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	op, err = s.actionsResolver.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	op, err = s.storageResolver.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	switch localState.Kind {
	case operation.RunHook:
		switch localState.Step {
		case operation.Pending:
			logger.Infof("awaiting error resolution for %q hook", localState.Hook.Kind)
			return s.nextOpHookError(localState, remoteState, opFactory)

		case operation.Queued:
			logger.Infof("found queued %q hook", localState.Hook.Kind)
			if localState.Hook.Kind == hooks.Install {
				// Special case: handle install in nextOp,
				// so we do nothing when the unit is dying.
				return s.nextOp(localState, remoteState, opFactory)
			}
			return opFactory.NewRunHook(*localState.Hook)

		case operation.Done:
			logger.Infof("committing %q hook", localState.Hook.Kind)
			return opFactory.NewSkipHook(*localState.Hook)

		default:
			return nil, errors.Errorf("unknown operation step %v", localState.Step)
		}

	case operation.Continue:
		logger.Infof("no operations in progress; waiting for changes")
		return s.nextOp(localState, remoteState, opFactory)

	default:
		return nil, errors.Errorf("unknown operation kind %v", localState.Kind)
	}
}

// nextOpConflicted is called after an upgrade operation has failed, and hasn't
// yet been resolved or reverted. When in this mode, the resolver will only
// consider those two possibilities for progressing.
func (s *uniterResolver) nextOpConflicted(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	if remoteState.ResolvedMode != params.ResolvedNone {
		if err := s.clearResolved(); err != nil {
			return nil, errors.Trace(err)
		}
		return opFactory.NewResolvedUpgrade(localState.CharmURL)
	}
	if remoteState.ForceCharmUpgrade && *localState.CharmURL != *remoteState.CharmURL {
		logger.Debugf("upgrade from %v to %v", localState.CharmURL, remoteState.CharmURL)
		return opFactory.NewRevertUpgrade(remoteState.CharmURL)
	}
	return nil, resolver.ErrWaiting
}

func (s *uniterResolver) nextOpHookError(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {

	// Report the hook error.
	if err := s.reportHookError(*localState.Hook); err != nil {
		return nil, errors.Trace(err)
	}

	if remoteState.ForceCharmUpgrade && *localState.CharmURL != *remoteState.CharmURL {
		logger.Debugf("upgrade from %v to %v", localState.CharmURL, remoteState.CharmURL)
		return opFactory.NewUpgrade(remoteState.CharmURL)
	}

	switch remoteState.ResolvedMode {
	case params.ResolvedNone:
		return nil, resolver.ErrNoOperation
	case params.ResolvedRetryHooks:
		if err := s.clearResolved(); err != nil {
			return nil, errors.Trace(err)
		}
		return opFactory.NewRunHook(*localState.Hook)
	case params.ResolvedNoHooks:
		if err := s.clearResolved(); err != nil {
			return nil, errors.Trace(err)
		}
		return opFactory.NewSkipHook(*localState.Hook)
	default:
		return nil, errors.Errorf(
			"unknown resolved mode %q", remoteState.ResolvedMode,
		)
	}
}

func (s *uniterResolver) nextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	switch remoteState.Life {
	case params.Alive:
	case params.Dying:
		// Normally we handle relations last, but if we're dying we
		// must ensure that all relations are broken first.
		op, err := s.relationsResolver.NextOp(localState, remoteState, opFactory)
		if errors.Cause(err) != resolver.ErrNoOperation {
			return op, err
		}

		// We're not in a hook error and the unit is Dying,
		// so we should proceed to tear down.
		//
		// TODO(axw) move logic for cascading destruction of
		//           subordinates, relation units and storage
		//           attachments into state, via cleanups.
		if localState.Started {
			return opFactory.NewRunHook(hook.Info{Kind: hooks.Stop})
		}
		fallthrough
	case params.Dead:
		// The unit is dying/dead and stopped, so tell the uniter
		// to terminate.
		return nil, resolver.ErrTerminate
	}

	// Now that storage hooks have run at least once, before anything else,
	// we need to run the install hook.
	if !localState.Installed {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.Install})
	}

	if *localState.CharmURL != *remoteState.CharmURL {
		logger.Debugf("upgrade from %v to %v", localState.CharmURL, remoteState.CharmURL)
		return opFactory.NewUpgrade(remoteState.CharmURL)
	}

	if localState.ConfigVersion != remoteState.ConfigVersion {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	}

	op, err := s.relationsResolver.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	// UpdateStatus hook runs if nothing else needs to.
	if remoteState.UpdateStatusRequired {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
	}

	return nil, resolver.ErrNoOperation
}
