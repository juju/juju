// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	jujucharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/wrench"
	"github.com/juju/juju/rpc/params"
)

// ResolverConfig defines configuration for the uniter resolver.
type ResolverConfig struct {
	ModelType           model.ModelType
	ClearResolved       func() error
	ReportHookError     func(hook.Info) error
	ShouldRetryHooks    bool
	StartRetryHookTimer func()
	StopRetryHookTimer  func()
	VerifyCharmProfile  resolver.Resolver
	UpgradeSeries       resolver.Resolver
	Reboot              resolver.Resolver
	Leadership          resolver.Resolver
	Actions             resolver.Resolver
	CreatedRelations    resolver.Resolver
	Relations           resolver.Resolver
	Storage             resolver.Resolver
	Commands            resolver.Resolver
	Secrets             resolver.Resolver
	OptionalResolvers   []resolver.Resolver
	Logger              logger.Logger
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
	ctx stdcontext.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (_ operation.Operation, err error) {
	badge := "<unspecified>"
	defer func() {
		if err != nil && errors.Cause(err) != resolver.ErrNoOperation && err != resolver.ErrRestart {
			s.config.Logger.Debugf("next %q operation could not be resolved: %v", badge, err)
		}
	}()

	if remoteState.Life == life.Dead || localState.Removed {
		return nil, resolver.ErrUnitDead
	}
	log := s.config.Logger

	// Operations for series-upgrade need to be resolved early,
	// in particular because no other operations should be run when the unit
	// has completed preparation and is waiting for upgrade completion.
	badge = "upgrade series"
	op, err := s.config.UpgradeSeries.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		if errors.Cause(err) == resolver.ErrDoNotProceed {
			return nil, resolver.ErrNoOperation
		}
		return op, err
	}

	// Check if we need to notify the charms because a reboot was detected.
	badge = "reboot"
	op, err = s.config.Reboot.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	if localState.Kind == operation.Upgrade {
		badge = "upgrade"
		if localState.Conflicted {
			return s.nextOpConflicted(ctx, localState, remoteState, opFactory)
		}
		// continue upgrading the charm
		log.Infof("resuming charm upgrade")
		return s.newUpgradeOperation(ctx, localState, remoteState, opFactory)
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

	badge = "relations"
	op, err = s.config.CreatedRelations.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	badge = "leadership"
	op, err = s.config.Leadership.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	badge = "optional"
	for _, r := range s.config.OptionalResolvers {
		op, err = r.NextOp(ctx, localState, remoteState, opFactory)
		if errors.Cause(err) != resolver.ErrNoOperation {
			return op, err
		}
	}

	badge = "secrets"
	op, err = s.config.Secrets.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	badge = "actions"
	op, err = s.config.Actions.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	badge = "commands"
	op, err = s.config.Commands.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	badge = "storage"
	op, err = s.config.Storage.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	// If we are to shut down, we don't want to start running any more queued/pending hooks.
	if remoteState.Shutdown {
		badge = "shutdown"
		log.Debugf("unit agent is shutting down, will not run pending/queued hooks")
		return s.nextOp(ctx, localState, remoteState, opFactory)
	}

	switch localState.Kind {
	case operation.RunHook:
		step := localState.Step
		if localState.HookStep != nil {
			step = *localState.HookStep
		}
		switch step {
		case operation.Pending:
			badge = "resolve hook"
			log.Infof("awaiting error resolution for %q hook", localState.Hook.Kind)
			return s.nextOpHookError(ctx, localState, remoteState, opFactory)

		case operation.Queued:
			badge = "queued hook"
			log.Infof("found queued %q hook", localState.Hook.Kind)
			if localState.Hook.Kind == hooks.Install {
				// Special case: handle install in nextOp,
				// so we do nothing when the unit is dying.
				return s.nextOp(ctx, localState, remoteState, opFactory)
			}
			return opFactory.NewRunHook(*localState.Hook)

		case operation.Done:
			// Only check for the wrench if trace logging is enabled. Otherwise,
			// we'd have to parse the charm url every time just to check to see
			// if a wrench existed.
			badge = "commit hook"
			if localState.CharmURL != "" && log.IsLevelEnabled(logger.TRACE) {
				// If it's set, the charm url will parse.
				curl := jujucharm.MustParseURL(localState.CharmURL)
				if curl != nil && wrench.IsActive("hooks", fmt.Sprintf("%s-%s-error", curl.Name, localState.Hook.Kind)) {
					s.config.Logger.Errorf("commit hook %q failed due to a wrench in the works", localState.Hook.Kind)
					return nil, errors.Errorf("commit hook %q failed due to a wrench in the works", localState.Hook.Kind)
				}
			}

			log.Infof("committing %q hook", localState.Hook.Kind)
			return opFactory.NewSkipHook(*localState.Hook)

		default:
			return nil, errors.Errorf("unknown hook operation step %v", step)
		}

	case operation.Continue:
		badge = "idle"
		log.Debugf("no operations in progress; waiting for changes")
		return s.nextOp(ctx, localState, remoteState, opFactory)

	default:
		return nil, errors.Errorf("unknown operation kind %v", localState.Kind)
	}
}

// nextOpConflicted is called after an upgrade operation has failed, and hasn't
// yet been resolved or reverted. When in this mode, the resolver will only
// consider those two possibilities for progressing.
func (s *uniterResolver) nextOpConflicted(
	ctx stdcontext.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	// Only IAAS models deal with conflicted upgrades.
	// TODO(caas) - what to do here.

	// Verify the charm profile before proceeding.  No hooks to run, if the
	// correct one is not yet applied.
	_, err := s.config.VerifyCharmProfile.NextOp(ctx, localState, remoteState, opFactory)
	if e := errors.Cause(err); e == resolver.ErrDoNotProceed {
		return nil, resolver.ErrNoOperation
	} else if e != resolver.ErrNoOperation {
		return nil, err
	}

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

func (s *uniterResolver) newUpgradeOperation(
	ctx stdcontext.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	// Verify the charm profile before proceeding.  No hooks to run, if the
	// correct one is not yet applied.
	_, err := s.config.VerifyCharmProfile.NextOp(ctx, localState, remoteState, opFactory)
	if e := errors.Cause(err); e == resolver.ErrDoNotProceed {
		return nil, resolver.ErrNoOperation
	} else if e != resolver.ErrNoOperation {
		return nil, err
	}
	return opFactory.NewUpgrade(remoteState.CharmURL)
}

func (s *uniterResolver) nextOpHookError(
	ctx stdcontext.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {

	// Report the hook error.
	if err := s.config.ReportHookError(*localState.Hook); err != nil {
		return nil, errors.Trace(err)
	}

	if remoteState.ForceCharmUpgrade && s.charmModified(localState, remoteState) {
		return s.newUpgradeOperation(ctx, localState, remoteState, opFactory)
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
	if remote.CharmURL == "" {
		return false
	}
	if local.CharmURL != remote.CharmURL {
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
	ctx stdcontext.Context,
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	switch remoteState.Life {
	case life.Alive:
		if remoteState.Shutdown {
			if localState.Started && !localState.Stopped {
				return opFactory.NewRunHook(hook.Info{Kind: hooks.Stop})
			} else if !localState.Started || localState.Stopped {
				return nil, worker.ErrTerminateAgent
			}
		}
	case life.Dying:
		// Normally we handle relations last, but if we're dying we
		// must ensure that all relations are broken first.
		op, err := s.config.Relations.NextOp(ctx, localState, remoteState, opFactory)
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
		return nil, resolver.ErrUnitDead
	}

	// Now that storage hooks have run at least once, before anything else,
	// we need to run the install hook.
	// TODO(cmars): remove !localState.Started. It's here as a temporary
	// measure because unit agent upgrades aren't being performed yet.
	if !localState.Installed && !localState.Started {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.Install})
	}

	if s.charmModified(localState, remoteState) {
		return s.newUpgradeOperation(ctx, localState, remoteState, opFactory)
	}

	configHashChanged := localState.ConfigHash != remoteState.ConfigHash
	trustHashChanged := localState.TrustHash != remoteState.TrustHash
	addressesHashChanged := localState.AddressesHash != remoteState.AddressesHash
	if configHashChanged || trustHashChanged || addressesHashChanged {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	}

	op, err := s.config.Relations.NextOp(ctx, localState, remoteState, opFactory)
	if errors.Cause(err) != resolver.ErrNoOperation {
		return op, err
	}

	// UpdateStatus hook runs if nothing else needs to.
	if localState.UpdateStatusVersion != remoteState.UpdateStatusVersion {
		return opFactory.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
	}

	return nil, resolver.ErrNoOperation
}
