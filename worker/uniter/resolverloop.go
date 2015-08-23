package uniter

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

// resolverLoopConfig contains configuration parameters for the resolver
// loop.
type resolverLoopConfig struct {
	resolver            resolver.Resolver
	remoteStateWatcher  remotestate.Watcher
	executor            operation.Executor
	factory             operation.Factory
	updateStatusChannel func() <-chan time.Time
	charmURL            *charm.URL
	dying               <-chan struct{}
	onIdle              func() error
}

func resolverLoop(config resolverLoopConfig) (resolver.LocalState, error) {
	rf := &resolver.OpFactory{
		Factory: config.factory,
		LocalState: resolver.LocalState{
			CharmURL: config.charmURL,
			Upgraded: false,
		},
	}
	for {
		updateStatus := config.updateStatusChannel()
		rf.RemoteState = config.remoteStateWatcher.Snapshot()
		rf.LocalState.State = config.executor.State()

		op, err := config.resolver.NextOp(rf.LocalState, rf.RemoteState, rf)
		for err == nil {
			logger.Tracef("running op: %v", op)
			if err := config.executor.Run(op); err != nil {
				return rf.LocalState, errors.Trace(err)
			}
			// Refresh snapshot, in case remote state
			// changed between operations.
			rf.RemoteState = config.remoteStateWatcher.Snapshot()
			rf.LocalState.State = config.executor.State()
			op, err = config.resolver.NextOp(rf.LocalState, rf.RemoteState, rf)
		}

		switch errors.Cause(err) {
		case nil:
		case resolver.ErrWaiting:
			// If a resolver is waiting for events to
			// complete, the agent is not idle.
		case resolver.ErrNoOperation:
			if err := config.onIdle(); err != nil {
				return rf.LocalState, errors.Trace(err)
			}
		default:
			return rf.LocalState, err
		}

		select {
		case <-config.dying:
			return rf.LocalState, tomb.ErrDying
		case <-config.remoteStateWatcher.RemoteStateChanged():
		case <-updateStatus:
			op, err := config.factory.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
			if err != nil {
				return rf.LocalState, errors.Trace(err)
			}
			if err := config.executor.Run(op); err != nil {
				return rf.LocalState, errors.Trace(err)
			}
		}
	}
}
