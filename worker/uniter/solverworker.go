package uniter

import (
	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/solver"
)

type solverWorker struct {
	tomb     tomb.Tomb
	watcher  remotestate.Watcher
	solver   solver.Solver
	executor operation.Executor
}

func NewSolverWorker(
	watcher remotestate.Watcher,
	solver solver.Solver,
	executor operation.Executor,
) *solverWorker {
	w := &solverWorker{
		watcher: watcher,
		solver:  solver,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *solverWorker) loop() error {
	for {
		remoteState := w.watcher.Snapshot()
		op := w.solver.NextOp(remoteState)
		for op != nil {
			if err := w.executor.Run(op); err != nil {
				return errors.Trace(err)
			}
			op = w.solver.NextOp(remoteState)
		}
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.watcher.RemoteStateChanged():
			// TODO(axw) !ok => dying
			if !ok {
				panic("!ok")
			}
			remoteState = w.watcher.Snapshot()
		}
	}
}
