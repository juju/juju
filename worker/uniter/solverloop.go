package uniter

import (
	"time"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/solver"
)

func solverLoop(
	s solver.Solver,
	w remotestate.Watcher,
	e operation.Executor,
	dying <-chan struct{},
	onIdle func() error,
) error {
	for {
		remoteState := w.Snapshot()
		op, err := s.NextOp(e.State(), remoteState)
		for err != solver.ErrNoOperation {
			if err != nil {
				return errors.Trace(err)
			}
			if err := e.Run(op); err != nil {
				return errors.Trace(err)
			}
			op, err = s.NextOp(e.State(), remoteState)
		}

		select {
		case <-dying:
			return tomb.ErrDying
		case <-time.After(idleWaitTime):
			// TODO(axw) pass in Clock for time.After
			if err := onIdle(); err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-w.RemoteStateChanged():
			// TODO(axw) !ok => dying
			if !ok {
				panic("!ok")
			}
			remoteState = w.Snapshot()
		}
	}
}
