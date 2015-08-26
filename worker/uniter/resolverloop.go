package uniter

import (
	"time"

	"github.com/juju/errors"
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
	dying               <-chan struct{}
	onIdle              func() error
}

func resolverLoop(config resolverLoopConfig) error {
	for {
		updateStatus := config.updateStatusChannel()

		remoteState := config.remoteStateWatcher.Snapshot()
		op, err := config.resolver.NextOp(config.executor.State(), remoteState)
		for err == nil {
			logger.Tracef("running op: %v", op)
			if err := config.executor.Run(op); err != nil {
				return errors.Trace(err)
			}
			// Refresh snapshot, in case remote state
			// changed between operations.
			remoteState = config.remoteStateWatcher.Snapshot()
			op, err = config.resolver.NextOp(config.executor.State(), remoteState)
		}

		switch errors.Cause(err) {
		case nil:
		case resolver.ErrWaiting:
			// If a resolver is waiting for events to
			// complete, the agent is not idle.
		case resolver.ErrNoOperation:
			if err := config.onIdle(); err != nil {
				return errors.Trace(err)
			}
		default:
			return err
		}

		select {
		case <-config.dying:
			return tomb.ErrDying
		case _, ok := <-config.remoteStateWatcher.RemoteStateChanged():
			// TODO(axw) !ok => dying
			if !ok {
				panic("!ok")
			}
		case <-updateStatus:
			op, err := config.factory.NewRunHook(hook.Info{Kind: hooks.UpdateStatus})
			if err != nil {
				return errors.Trace(err)
			}
			if err := config.executor.Run(op); err != nil {
				return errors.Trace(err)
			}
		}
	}
}
