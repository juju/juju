// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package terminationworker

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/juju/worker/v2"
	"gopkg.in/tomb.v2"

	jworker "github.com/juju/juju/worker"
)

// TerminationSignal is the signal that
// indicates the agent should terminate
// and uninstall itself.
//
// We do not use SIGTERM as SIGTERM is the
// default signal used to initiate a graceful
// shutdown.
const TerminationSignal = syscall.SIGABRT

type terminationWorker struct {
	tomb tomb.Tomb
}

// NewWorker returns a worker that waits for a
// TerminationSignal signal, and then exits
// with worker.ErrTerminateAgent.
func NewWorker() worker.Worker {
	var w terminationWorker
	c := make(chan os.Signal, 1)
	signal.Notify(c, TerminationSignal)
	w.tomb.Go(func() error {
		defer signal.Stop(c)
		return w.loop(c)
	})
	return &w
}

func (w *terminationWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *terminationWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *terminationWorker) loop(c <-chan os.Signal) (err error) {
	select {
	case <-c:
		return jworker.ErrTerminateAgent
	case <-w.tomb.Dying():
		return tomb.ErrDying
	}
}
