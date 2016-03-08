// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

// ErrLoopAborted is used to signal that the loop is exiting because it
// received a value on its config's Abort chan.
var ErrLoopAborted = errors.New("resolver loop aborted")

// LoopConfig contains configuration parameters for the resolver loop.
type LoopConfig struct {
	Resolver      Resolver
	Watcher       remotestate.Watcher
	Executor      operation.Executor
	Factory       operation.Factory
	Abort         <-chan struct{}
	OnIdle        func() error
	CharmDirGuard fortress.Guard
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
//  - if the "abort" channel is signalled, then the loop will
//    exit with ErrLoopAborted
//  - if the resolver returns ErrWaiting, then no operations
//    will be executed until the remote state has changed
//    again
//  - if the resolver returns ErrNoOperation, then "onIdle"
//    will be invoked and the loop will wait until the remote
//    state has changed again
//  - if the resolver, onIdle, or executor return some other
//    error, the loop will exit immediately
func Loop(cfg LoopConfig, localState *LocalState) error {
	rf := &resolverOpFactory{Factory: cfg.Factory, LocalState: localState}

	// Initialize charmdir availability before entering the loop in case we're recovering from a restart.
	err := updateCharmDir(cfg.Executor.State(), cfg.CharmDirGuard, cfg.Abort)
	if err != nil {
		return errors.Trace(err)
	}

	for {
		rf.RemoteState = cfg.Watcher.Snapshot()
		rf.LocalState.State = cfg.Executor.State()

		op, err := cfg.Resolver.NextOp(*rf.LocalState, rf.RemoteState, rf)
		for err == nil {
			logger.Tracef("running op: %v", op)
			if err := cfg.Executor.Run(op); err != nil {
				return errors.Trace(err)
			}
			// Refresh snapshot, in case remote state
			// changed between operations.
			rf.RemoteState = cfg.Watcher.Snapshot()
			rf.LocalState.State = cfg.Executor.State()

			err = updateCharmDir(rf.LocalState.State, cfg.CharmDirGuard, cfg.Abort)
			if err != nil {
				return errors.Trace(err)
			}

			op, err = cfg.Resolver.NextOp(*rf.LocalState, rf.RemoteState, rf)
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
		case <-cfg.Abort:
			return ErrLoopAborted
		case <-cfg.Watcher.RemoteStateChanged():
		}
	}
}

// updateCharmDir sets charm directory availability for sharing among
// concurrent workers according to local operation state.
func updateCharmDir(opState operation.State, guard fortress.Guard, abort fortress.Abort) error {
	var changing bool

	// Determine if the charm content is changing.
	if opState.Kind == operation.Install || opState.Kind == operation.Upgrade {
		changing = true
	} else if opState.Kind == operation.RunHook && opState.Hook != nil && opState.Hook.Kind == hooks.UpgradeCharm {
		changing = true
	}

	available := opState.Started && !opState.Stopped && !changing
	logger.Tracef("charmdir: available=%v opState: started=%v stopped=%v changing=%v",
		available, opState.Started, opState.Stopped, changing)
	if available {
		return guard.Unlock()
	} else {
		return guard.Lockdown(abort)
	}
}
