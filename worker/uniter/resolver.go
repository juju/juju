// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

// ResolverConfig defines configuration for the uniter resolver.
type ResolverConfig struct {
	ModelType           model.ModelType
	ClearResolved       func() error
	ReportHookError     func(hook.Info) error
	ShouldRetryHooks    bool
	StartRetryHookTimer func()
	StopRetryHookTimer  func()
	UpgradeSeries       resolver.Resolver
	Leadership          resolver.Resolver
	Actions             resolver.Resolver
	CreatedRelations    resolver.Resolver
	Relations           resolver.Resolver
	Storage             resolver.Resolver
	Commands            resolver.Resolver
	Container           resolver.Resolver
	Logger              Logger
}

type uniterResolver struct {
	config                ResolverConfig
	retryHookTimerStarted bool
}

// NewUniterResolver returns a new resolver.Resolver for the uniter.
func NewUniterResolver(cfg ResolverConfig) resolver.Resolver {
	return &uniterResolver{
		config:                cfg,
		retryHookTimerStarted: false,
	}
}

func (s *uniterResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	if remoteState.Life == life.Dead || localState.Removed {
		return nil, resolver.ErrTerminate
	}
	logger := s.config.Logger

	// Operations for series-upgrade need to be resolved early,
	// in particular because no other operations should be run when the unit
	// has completed preparation and is waiting for upgrade completion.
	op, err := s.config.UpgradeSeries.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		if errors.Cause(err) == resolver.ErrDoNotProceed {
			return nil, resolver.ErrNoOperation
		}
		return op, err
	}

	if localState.Kind == operation.Upgrade {
		if localState.Conflicted {
			return s.nextOpConflicted(localState, remoteState, opFactory)
		}
		// continue upgrading the charm
		logger.Infof("resuming charm upgrade")
		return opFactory.NewUpgrade(localState.CharmURL)
	}

	if localState.Restart {
		// We've just run the upgrade op, which will change the
		// unit's charm URL. We need to restart the resolver
		// loop so that we start watching the correct events.
		return nil, resolver.ErrRestart
	}

	if s.retryHookTimerStarted && (localState.Kind != operation.RunHook || localState.Step != operation.Pending) {
		// The hook-retry timer is running, but there is no pending
		// hook operation. We're not in an error state, so stop the
		// timer now to reset the backoff state.
		s.config.StopRetryHookTimer()
		s.retryHookTimerStarted = false
	}

	op, err = s.config.CreatedRelations.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	op, err = s.config.Leadership.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	if s.config.Container != nil {
		op, err = s.config.Container.NextOp(localState, remoteState, opFactory)
		if errors.Cause(err) != resolver.ErrNoOperation {
			return op, err
		}
	}

	op, err = s.config.Actions.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	op, err = s.config.Commands.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	op, err = s.config.Storage.NextOp(localState, remoteState, opFactory)
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
		logger.Debugf("no operations in progress; waiting for changes")
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
	// Only IAAS models deal with conflicted upgrades.
	// TODO(caas) - what to do here.
	if remoteState.ResolvedMode != params.ResolvedNone {
		if err := s.config.ClearResolved(); err != nil {
			return nil, errors.Trace(err)
		}
		return opFactory.NewResolvedUpgrade(localState.CharmURL)
	}
	if remoteState.ForceCharmUpgrade && s.charmModified(localState, remoteState) {
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
	if err := s.config.ReportHookError(*localState.Hook); err != nil {
		return nil, errors.Trace(err)
	}

	if remoteState.ForceCharmUpgrade && s.charmModified(localState, remoteState) {
		return opFactory.NewUpgrade(remoteState.CharmURL)
	}

	switch remoteState.ResolvedMode {
	case params.ResolvedNone:
		if remoteState.RetryHookVersion > localState.RetryHookVersion {
			// We've been asked to retry: clear the hook timer
			// started state so we'll restart it if this fails.
			//
			// If the hook fails again, we'll re-enter this method
			// with the retry hook versions equal and restart the
			// timer. If the hook succeeds, we'll enter nextOp
			// and stop the timer.
			s.retryHookTimerStarted = false
			return opFactory.NewRunHook(*localState.Hook)
		}
		if !s.retryHookTimerStarted && s.config.ShouldRetryHooks {
			// We haven't yet started a retry timer, so start one
			// now. If we retry and fail, retryHookTimerStarted is
			// cleared so that we'll still start it again.
			s.config.StartRetryHookTimer()
			s.retryHookTimerStarted = true
		}
		return nil, resolver.ErrNoOperation
	case params.ResolvedRetryHooks:
		s.config.StopRetryHookTimer()
		s.retryHookTimerStarted = false
		if err := s.config.ClearResolved(); err != nil {
			return nil, errors.Trace(err)
		}
		return opFactory.NewRunHook(*localState.Hook)
	case params.ResolvedNoHooks:
		s.config.StopRetryHookTimer()
		s.retryHookTimerStarted = false
		if err := s.config.ClearResolved(); err != nil {
			return nil, errors.Trace(err)
		}
		return opFactory.NewSkipHook(*localState.Hook)
	default:
		return nil, errors.Errorf(
			"unknown resolved mode %q", remoteState.ResolvedMode,
		)
	}
}

func (s *uniterResolver) charmModified(local resolver.LocalState, remote remotestate.Snapshot) bool {
	// CAAS models may not yet have read the charm url from state.
	if remote.CharmURL == nil {
		return false
	}
	if *local.CharmURL != *remote.CharmURL {
		s.config.Logger.Debugf("upgrade from %v to %v", local.CharmURL, remote.CharmURL)
		return true
	}

	if local.CharmModifiedVersion != remote.CharmModifiedVersion {
		s.config.Logger.Debugf("upgrade from CharmModifiedVersion %v to %v", local.CharmModifiedVersion, remote.CharmModifiedVersion)
		return true
	}
	return false
}

func (s *uniterResolver) nextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	switch remoteState.Life {
	case life.Alive:
	case life.Dying:
		// Normally we handle relations last, but if we're dying we
		// must ensure that all relations are broken first.
		op, err := s.config.Relations.NextOp(localState, remoteState, opFactory)
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
		} else if localState.Installed && !localState.Removed {
			return opFactory.NewRunHook(hook.Info{Kind: hooks.Remove})
		}
		fallthrough
	case life.Dead:
		// The unit is dying/dead and stopped, so tell the uniter
		// to terminate.
		return nil, resolver.ErrTerminate
	}

	// Now that storage hooks have run at least once, before anything else,
	// we need to run the install hook.
	// TODO(cmars): remove !localState.Started. It's here as a temporary
	// measure because unit agent upgrades aren't being performed yet.
	if !localState.Installed && !localState.Started {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.Install})
	}

	if s.charmModified(localState, remoteState) {
		return opFactory.NewUpgrade(remoteState.CharmURL)
	}

	configHashChanged := localState.ConfigHash != remoteState.ConfigHash
	trustHashChanged := localState.TrustHash != remoteState.TrustHash
	addressesHashChanged := localState.AddressesHash != remoteState.AddressesHash
	if configHashChanged || trustHashChanged || addressesHashChanged {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	}

	op, err := s.config.Relations.NextOp(localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	// UpdateStatus hook runs if nothing else needs to.
	if localState.UpdateStatusVersion != remoteState.UpdateStatusVersion {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
	}

	return nil, resolver.ErrNoOperation
}
