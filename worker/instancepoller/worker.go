// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
)

type updaterWorker struct {
	st   *state.State
	tomb tomb.Tomb
	*aggregator

	observer *worker.EnvironObserver
}

// NewWorker returns a worker that keeps track of
// the machines in the state and polls their instance
// addresses and status periodically to keep them up to date.
func NewWorker(st *state.State) worker.Worker {
	u := &updaterWorker{
		st: st,
	}
	// wait for environment
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop())
	}()
	return u
}

func (u *updaterWorker) Kill() {
	u.tomb.Kill(nil)
}

func (u *updaterWorker) Wait() error {
	return u.tomb.Wait()
}

func (u *updaterWorker) loop() (err error) {
	u.observer, err = worker.NewEnvironObserver(u.st)
	if err != nil {
		return err
	}
	u.aggregator = newAggregator(u.observer.Environ())
	logger.Infof("instance poller received inital environment configuration")
	defer func() {
		obsErr := worker.Stop(u.observer)
		if err == nil {
			err = obsErr
		}
	}()
	return watchMachinesLoop(u, u.st.WatchEnvironMachines())
}

func (u *updaterWorker) newMachineContext() machineContext {
	return u
}

func (u *updaterWorker) getMachine(id string) (machine, error) {
	return u.st.Machine(id)
}

func (u *updaterWorker) dying() <-chan struct{} {
	return u.tomb.Dying()
}

func (u *updaterWorker) killAll(err error) {
	u.tomb.Kill(err)
}
