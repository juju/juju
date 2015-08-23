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

	// TODO(axw) move this to LocalState
	conflicted bool

	leadershipResolver resolver.Resolver
	relationsResolver  resolver.Resolver
	storageResolver    resolver.Resolver
}

func (s *uniterResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {

	if localState.Kind == operation.Upgrade {
		// TODO(axw) double check this. I think we should
		// be calling NewRevertUpgrade or whatever?
		if s.conflicted {
			if remoteState.ResolvedMode == params.ResolvedNone {
				return nil, resolver.ErrNoOperation
			}
			if err := s.clearResolved(); err != nil {
				return nil, errors.Trace(err)
			}
			s.conflicted = false
		}
		logger.Infof("resuming charm upgrade")
		return opFactory.NewUpgrade(localState.CharmURL)
	}

	if localState.Upgraded {
		// We've just run the upgrade op, which will change the
		// unit's charm URL. We need to restart the resolver
		// loop so that we start watching the correct events.
		return nil, resolver.ErrRestart
	}

	op, err := s.leadershipResolver.NextOp(localState, remoteState, opFactory)
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
		if localState.Started && !localState.Stopped {
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

	return s.relationsResolver.NextOp(localState, remoteState, opFactory)
}
