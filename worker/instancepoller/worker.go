// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/environ"
)

type updaterWorker struct {
	st         *instancepoller.API
	aggregator *aggregator
	catacomb   catacomb.Catacomb
}

// NewWorker returns a worker that keeps track of
// the machines in the state and polls their instance
// addresses and status periodically to keep them up to date.
func NewWorker(st *instancepoller.API) (worker.Worker, error) {
	u := &updaterWorker{
		st: st,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: u.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

// Kill is part of the worker.Worker interface.
func (u *updaterWorker) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *updaterWorker) Wait() error {
	return u.catacomb.Wait()
}

func (u *updaterWorker) loop() (err error) {

	// TODO(fwereade): get this as a resource from a dependency.Engine.
	tracker, err := environ.NewTracker(environ.Config{
		Observer: u.st,
	})
	if err != nil {
		return errors.Trace(err)
	}
	if err := u.catacomb.Add(tracker); err != nil {
		return errors.Trace(err)
	}
	u.aggregator = newAggregator(tracker.Environ())
	if err := u.catacomb.Add(u.aggregator); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("instance poller received inital environment configuration")

	watcher, err := u.st.WatchModelMachines()
	if err != nil {
		return err
	}
	if err := u.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}
	return watchMachinesLoop(u, watcher)
}

// newMachineContext is part of the updaterContext interface.
func (u *updaterWorker) newMachineContext() machineContext {
	return u
}

// getMachine is part of the machineContext interface.
func (u *updaterWorker) getMachine(tag names.MachineTag) (machine, error) {
	return u.st.Machine(tag)
}

// instanceInfo is part of the machineContext interface.
func (u *updaterWorker) instanceInfo(id instance.Id) (instanceInfo, error) {
	return u.aggregator.instanceInfo(id)
}

// kill is part of the lifetimeContext interface.
func (u *updaterWorker) kill(err error) {
	u.catacomb.Kill(err)
}

// dying is part of the lifetimeContext interface.
func (u *updaterWorker) dying() <-chan struct{} {
	return u.catacomb.Dying()
}

// errDying is part of the lifetimeContext interface.
func (u *updaterWorker) errDying() error {
	return u.catacomb.ErrDying()
}
