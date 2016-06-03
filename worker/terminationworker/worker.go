// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package terminationworker

import (
	"os"
	"os/signal"
	"syscall"

	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
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
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return &w
}

func (w *terminationWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *terminationWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *terminationWorker) loop() (err error) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, TerminationSignal)
	defer signal.Stop(c)
	select {
	case <-c:
		return worker.ErrTerminateAgent
	case <-w.tomb.Dying():
		return tomb.ErrDying
	}
}
