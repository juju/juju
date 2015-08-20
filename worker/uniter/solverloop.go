package uniter

import (
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
		for err != solver.ErrNoOperation && err != solver.ErrWaiting {
			if err != nil {
				return errors.Trace(err)
			}
			if err := e.Run(op); err != nil {
				return errors.Trace(err)
			}
			// Refresh snapshot, in case remote state
			// changed between operations.
			remoteState = w.Snapshot()
			op, err = s.NextOp(e.State(), remoteState)
		}

		// If a solver is waiting for events to complete, the
		// agent is not idle.
		if err != solver.ErrWaiting {
			if err := onIdle(); err != nil {
				return errors.Trace(err)
			}
		}

		select {
		case <-dying:
			return tomb.ErrDying
		case _, ok := <-w.RemoteStateChanged():
			// TODO(axw) !ok => dying
			if !ok {
				panic("!ok")
			}
		}
	}
}
