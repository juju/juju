// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package terminationworker

import (
	"os"
	"os/signal"
	"syscall"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/worker"
)

// TerminationSignal is the signal that
// indicates the agent should terminate
// and uninstall itself.
//
// We do not use SIGTERM as SIGTERM is the
// default signal used to initiate a graceful
// shutdown.
const TerminationSignal = syscall.SIGABRT

// NewWorker returns a worker that wais for a

type terminationWorker struct {
	tomb tomb.Tomb
}

// TerminationSignal signal and exits with
// ErrTerminateAgent.
func NewWorker() worker.Worker {
	u := &terminationWorker{}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop())
	}()
	return u
}

func (u *terminationWorker) Kill() {
	u.tomb.Kill(nil)
}

func (u *terminationWorker) Wait() error {
	return u.tomb.Wait()
}

func (u *terminationWorker) loop() (err error) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, TerminationSignal)
	defer signal.Stop(c)
	select {
	case <-c:
		return worker.ErrTerminateAgent
	case <-u.tomb.Dying():
		return nil
	}
}
