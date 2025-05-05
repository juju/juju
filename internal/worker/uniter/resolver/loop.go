// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"context"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mutex/v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	jujucharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

// ErrLoopAborted is used to signal that the loop is exiting because it
// received a value on its config's Abort chan.
var ErrLoopAborted = errors.New("resolver loop aborted")

// ErrDoNotProceed is used to distinguish behaviour from
// resolver.ErrNoOperation. i.e do not run any operations versus
// this resolver has no operations to run.
var ErrDoNotProceed = errors.New("do not proceed")

// LoopConfig contains configuration parameters for the resolver loop.
type LoopConfig struct {
	Resolver      Resolver
	Watcher       remotestate.Watcher
	Executor      operation.Executor
	Factory       operation.Factory
	OnIdle        func() error
	CharmDirGuard fortress.Guard
	CharmDir      string
	Logger        logger.Logger
}

// Loop repeatedly waits for remote state changes, feeding the local and
// remote state to the provided Resolver to generate Operations which are
// then run with the provided Executor.
//
// The provided "onIdle" function will be called when the loop is waiting
// for remote state changes due to a lack of work to perform. It will not
// be called when a change is anticipated (i.e. due to ErrWaiting).
//
// The resolver loop can be controlled in the following ways:
//   - if the "abort" channel is signalled, then the loop will
//     exit with ErrLoopAborted
//   - if the resolver returns ErrWaiting, then no operations
//     will be executed until the remote state has changed
//     again
//   - if the resolver returns ErrNoOperation, then "onIdle"
//     will be invoked and the loop will wait until the remote
//     state has changed again
//   - if the resolver, onIdle, or executor return some other
//     error, the loop will exit immediately
func Loop(ctx context.Context, cfg LoopConfig, localState *LocalState) error {
	rf := &resolverOpFactory{Factory: cfg.Factory, LocalState: localState}

	// Initialize charmdir availability before entering the loop in case we're recovering from a restart.
	err := updateCharmDir(ctx, cfg.Executor.State(), cfg.CharmDirGuard, cfg.Logger)
	if err != nil {
		return errors.Trace(err)
	}

	// If we're restarting the loop, ensure any pending charm upgrade is run
	// before continuing.
	err = checkCharmInstallUpgrade(ctx, cfg.Logger, cfg.CharmDir, cfg.Watcher.Snapshot(), rf, cfg.Executor)
	if err != nil {
		return errors.Trace(err)
	}

	fire := make(chan struct{}, 1)
	for {
		rf.RemoteState = cfg.Watcher.Snapshot()
		rf.LocalState.State = cfg.Executor.State()

		if localState.HookWasShutdown {
			agentShutdown := rf.RemoteState.Shutdown
			if !agentShutdown {
				agentShutdown = maybeAgentShutdown(cfg)
			}
			if !agentShutdown {
				cfg.Logger.Warningf(ctx, "last %q hook was killed, but agent still alive", localState.Hook.Kind)
			}
		}

		op, err := cfg.Resolver.NextOp(ctx, *rf.LocalState, rf.RemoteState, rf)
		for err == nil {
			// Send remote state changes to running operations.
			remoteStateChanged := make(chan remotestate.Snapshot)
			done := make(chan struct{})
			go func() {
				var rs chan remotestate.Snapshot
				for {
					select {
					case <-cfg.Watcher.RemoteStateChanged():
						// We consumed a remote state change event
						// so we need a way to trigger the select below
						// in case it was a new operation.
						select {
						case fire <- struct{}{}:
						default:
						}
						rs = remoteStateChanged
					case rs <- cfg.Watcher.Snapshot():
						rs = nil
					case <-done:
						return
					}
				}
			}()

			cfg.Logger.Tracef(ctx, "running op: %v", op)
			if err := cfg.Executor.Run(ctx, op, remoteStateChanged); err != nil {
				close(done)

				if errors.Cause(err) == mutex.ErrCancelled {
					// If the lock acquisition was cancelled (such as when the
					// migration-inactive flag drops) we do not want the
					// resolver to surface that error. This puts the agent into
					// the "failed" state, which causes the initial migration
					// validation phase to fail.
					// The safest thing to do is to bounce the loop and
					// reevaluate our state, which is what happens upon a
					// fortress error anyway (uniter.TranslateFortressErrors).
					cfg.Logger.Warningf(ctx, "executor lock acquisition cancelled")
					return ErrRestart
				}
				return errors.Trace(err)
			}
			close(done)

			// Refresh snapshot, in case remote state
			// changed between operations.
			rf.RemoteState = cfg.Watcher.Snapshot()
			rf.LocalState.State = cfg.Executor.State()

			err = updateCharmDir(ctx, rf.LocalState.State, cfg.CharmDirGuard, cfg.Logger)
			if err != nil {
				return errors.Trace(err)
			}

			op, err = cfg.Resolver.NextOp(ctx, *rf.LocalState, rf.RemoteState, rf)
		}

		switch errors.Cause(err) {
		case nil:
		case ErrWaiting:
			// If a resolver is waiting for events to
			// complete, the agent is not idle.
		case ErrNoOperation:
			if cfg.OnIdle != nil {
				if err := cfg.OnIdle(); err != nil {
					return errors.Trace(err)
				}
			}
		default:
			return err
		}

		select {
		case <-ctx.Done():
			return ErrLoopAborted
		case <-cfg.Watcher.RemoteStateChanged():
		case <-fire:
		}
	}
}

// maybeAgentShutdown returns true if the agent was killed by a
// SIGTERM. If not true at the time of calling, it will wait a short
// time for the status to possibly be updated.
func maybeAgentShutdown(cfg LoopConfig) bool {
	fire := make(chan struct{}, 1)
	remoteStateChanged := make(chan remotestate.Snapshot)
	done := make(chan struct{})
	defer close(done)
	go func() {
		var rs chan remotestate.Snapshot
		for {
			select {
			case <-cfg.Watcher.RemoteStateChanged():
				// We consumed a remote state change event
				// so we need a way to trigger the select below
				// in case it was a new operation.
				select {
				case fire <- struct{}{}:
				default:
				}
				rs = remoteStateChanged
			case rs <- cfg.Watcher.Snapshot():
				rs = nil
			case <-done:
				return
			}
		}
	}()
	for {
		select {
		case rs := <-remoteStateChanged:
			if rs.Shutdown {
				return true
			}
		case <-time.After(3 * time.Second):
			return false
		}
	}
}

// updateCharmDir sets charm directory availability for sharing among
// concurrent workers according to local operation state.
func updateCharmDir(ctx context.Context, opState operation.State, guard fortress.Guard, logger logger.Logger) error {
	var changing bool

	// Determine if the charm content is changing.
	if opState.Kind == operation.Install || opState.Kind == operation.Upgrade {
		changing = true
	} else if opState.Kind == operation.RunHook && opState.Hook != nil && opState.Hook.Kind == hooks.UpgradeCharm {
		changing = true
	}

	available := opState.Started && !opState.Stopped && !changing
	logger.Tracef(ctx, "charmdir: available=%v opState: started=%v stopped=%v changing=%v",
		available, opState.Started, opState.Stopped, changing)
	if available {
		return guard.Unlock(ctx)
	} else {
		return guard.Lockdown(ctx)
	}
}

func checkCharmInstallUpgrade(ctx context.Context, logger logger.Logger, charmDir string, remote remotestate.Snapshot, rf *resolverOpFactory, ex operation.Executor) error {
	// If we restarted due to error with a pending charm upgrade available,
	// do the upgrade now.  There are cases (lp:1895040) where the error was
	// caused because not all units were upgraded before relation-created
	// hooks were attempted for peer relations.  Do this before the remote
	// state watcher is started.  It will not trigger an upgrade, until the
	// next applicationChanged event.  Could get stuck in an error loop.

	local := rf.LocalState
	local.State = ex.State()

	opFunc := rf.NewUpgrade
	if !local.Installed && local.Hook != nil && local.Hook.Kind == hooks.Install && local.Step != operation.Done {
		// We must have failed to run the install hook, restarted (possibly in a sidecar charm), so need to re-run the install op.
		opFunc = rf.NewInstall
	} else if !local.Installed || remote.CharmURL == "" {
		// If the unit isn't installed, no need to start an upgrade.
		return nil
	}

	stat, err := os.Stat(charmDir)
	haveCharmDir := err == nil && stat.IsDir()
	if haveCharmDir {
		// If the unit is installed and already upgrading and the charm dir
		// exists no need to start an upgrade.
		if local.Kind == operation.Upgrade || (local.Hook != nil && local.Hook.Kind == hooks.UpgradeCharm) {
			return nil
		}
	}

	if local.Started && remote.CharmProfileRequired {
		if remote.LXDProfileName == "" {
			return nil
		}
		rev, err := lxdprofile.ProfileRevision(remote.LXDProfileName)
		if err != nil {
			return errors.Trace(err)
		}
		curl, err := jujucharm.ParseURL(remote.CharmURL)
		if err != nil {
			return errors.Trace(err)
		}
		if rev != curl.Revision {
			logger.Tracef(ctx, "Charm profile required: current revision %d does not match new revision %d", rev, curl.Revision)
			return nil
		}
	}

	sameCharm := local.CharmURL == remote.CharmURL
	if haveCharmDir && (!local.Started || sameCharm) {
		return nil
	}
	if !haveCharmDir {
		logger.Debugf(ctx, "start to re-download charm %v because charm dir %q has gone which is usually caused by operator pod re-scheduling", remote.CharmURL, charmDir)
	}
	if !sameCharm {
		logger.Debugf(ctx, "execute pending upgrade from %s to %s after uniter loop restart", local.CharmURL, remote.CharmURL)
	}

	op, err := opFunc(remote.CharmURL)
	if err != nil {
		return errors.Trace(err)
	}
	if err = ex.Run(ctx, op, nil); err != nil {
		return errors.Trace(err)
	}
	if local.Restart {
		return ErrRestart
	}
	return nil
}
