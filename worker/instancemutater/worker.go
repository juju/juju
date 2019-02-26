// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"
)

//go:generate mockgen -package mocks -destination mocks/instancebroker_mock.go github.com/juju/juju/worker/instancemutater InstanceBroker
//go:generate mockgen -package mocks -destination mocks/logger_mock.go github.com/juju/juju/worker/instancemutater Logger
//go:generate mockgen -package mocks -destination mocks/namestag_mock.go gopkg.in/juju/names.v2 Tag

type InstanceAPI interface {
	WatchModelMachines() (watcher.StringsWatcher, error)
	Machine(tag names.MachineTag) (machine, error)
}

// Logger represents the logging methods called.
type Logger interface {
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
}

// Config represents the configuration required to run a new instance mutater
// worker.
type Config struct {
	Facade InstanceAPI

	// Logger is the logger for this worker.
	Logger Logger

	// Tag is the current machine tag
	Tag names.Tag
}

// Validate checks for missing values from the configuration and checks that
// they conform to a given type.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Environ")
	}
	if config.Tag == nil {
		return errors.NotValidf("nil tag")
	}
	if _, ok := config.Tag.(names.MachineTag); !ok {
		return errors.NotValidf("tag")
	}
	return nil
}

// NewWorker returns a worker that keeps track of
// the machines/containers in the state and polls their instance
// for any changes.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	broker, ok := config.Facade.(environs.LXDProfiler)
	if !ok {

	}
	w := &mutaterWorker{
		logger:     config.Logger,
		broker:     broker,
		machineTag: config.Tag.(names.MachineTag),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type mutaterWorker struct {
	catacomb catacomb.Catacomb

	logger     Logger
	broker     environs.LXDProfiler
	machineTag names.MachineTag
	facade     InstanceAPI
}

func (w *mutaterWorker) loop() error {
	watcher, err := w.facade.WatchModelMachines()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}
	return watchMachinesLoop(w, watcher)
}

// Kill implements worker.Worker.Kill.
func (w *mutaterWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *mutaterWorker) Wait() error {
	return w.catacomb.Wait()
}

// Stop stops the upgradeseriesworker and returns any
// error it encountered when running.
func (w *mutaterWorker) Stop() error {
	w.Kill()
	return w.Wait()
}

// newMachineContext is part of the updaterContext interface.
func (w *mutaterWorker) newMachineContext() machineContext {
	return w
}

// getMachine is part of the machineContext interface.
func (w *mutaterWorker) getMachine(tag names.MachineTag) (machine, error) {
	return w.facade.Machine(tag)
}

// kill is part of the lifetimeContext interface.
func (w *mutaterWorker) kill(err error) {
	w.catacomb.Kill(err)
}

// dying is part of the lifetimeContext interface.
func (w *mutaterWorker) dying() <-chan struct{} {
	return w.catacomb.Dying()
}

// errDying is part of the lifetimeContext interface.
func (w *mutaterWorker) errDying() error {
	return w.catacomb.ErrDying()
}
