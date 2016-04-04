// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

type Config struct {
	Facade  *instancepoller.API
	Environ environs.Environ
}

func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Environ == nil {
		return errors.NotValidf("nil Environ")
	}
	return nil
}

type updaterWorker struct {
	config     Config
	aggregator *aggregator
	catacomb   catacomb.Catacomb
}

// NewWorker returns a worker that keeps track of
// the machines in the state and polls their instance
// addresses and status periodically to keep them up to date.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	u := &updaterWorker{
		config: config,
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
	u.aggregator = newAggregator(u.config.Environ)
	if err := u.catacomb.Add(u.aggregator); err != nil {
		return errors.Trace(err)
	}
	watcher, err := u.config.Facade.WatchModelMachines()
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
	return u.config.Facade.Machine(tag)
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
