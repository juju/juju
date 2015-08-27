// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

// LoopConfig contains configuration parameters for the resolver loop.
type LoopConfig struct {
	Resolver            Resolver
	Watcher             remotestate.Watcher
	Executor            operation.Executor
	Factory             operation.Factory
	UpdateStatusChannel func() <-chan time.Time
	CharmURL            *charm.URL
	Dying               <-chan struct{}
	OnIdle              func() error
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
//  - if the "dying" channel is signalled, then the loop will
//    exit with tomb.ErrDying
//  - if the resolver returns ErrWaiting, then no operations
//    will be executed until the remote state has changed
//    again
//  - if the resolver returns ErrNoOperation, then "onIdle"
//    will be invoked and the loop will wait until the remote
//    state has changed again
//  - if the resolver, onIdle, or executor return some other
//    error, the loop will exit immediately
//
// Loop will return the last LocalState acted upon, regardless of whether
// an error is returned. This can be used, for example, to obtain the
// charm URL being upgraded to.
func Loop(cfg LoopConfig) (LocalState, error) {
	rf := &resolverOpFactory{
		Factory:    cfg.Factory,
		LocalState: LocalState{CharmURL: cfg.CharmURL},
	}
	for {
		// TODO(axw) move update status to the watcher.
		updateStatus := cfg.UpdateStatusChannel()
		rf.RemoteState = cfg.Watcher.Snapshot()
		rf.LocalState.State = cfg.Executor.State()

		op, err := cfg.Resolver.NextOp(rf.LocalState, rf.RemoteState, rf)
		for err == nil {
			logger.Tracef("running op: %v", op)
			if err := cfg.Executor.Run(op); err != nil {
				return rf.LocalState, errors.Trace(err)
			}
			// Refresh snapshot, in case remote state
			// changed between operations.
			rf.RemoteState = cfg.Watcher.Snapshot()
			rf.LocalState.State = cfg.Executor.State()
			op, err = cfg.Resolver.NextOp(rf.LocalState, rf.RemoteState, rf)
		}

		switch errors.Cause(err) {
		case nil:
		case ErrWaiting:
			// If a resolver is waiting for events to
			// complete, the agent is not idle.
		case ErrNoOperation:
			if cfg.OnIdle != nil {
				if err := cfg.OnIdle(); err != nil {
					return rf.LocalState, errors.Trace(err)
				}
			}
		default:
			return rf.LocalState, err
		}

		select {
		case <-cfg.Dying:
			return rf.LocalState, tomb.ErrDying
		case <-cfg.Watcher.RemoteStateChanged():
		case <-updateStatus:
			// TODO(axw) move this to a resolver.
			op, err := cfg.Factory.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
			if err != nil {
				return rf.LocalState, errors.Trace(err)
			}
			if err := cfg.Executor.Run(op); err != nil {
				return rf.LocalState, errors.Trace(err)
			}
		}
	}
}
