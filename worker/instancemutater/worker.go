// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
)

//go:generate mockgen -package mocks -destination mocks/instancebroker_mock.go github.com/juju/juju/worker/instancemutater InstanceMutaterAPI
//go:generate mockgen -package mocks -destination mocks/logger_mock.go github.com/juju/juju/worker/instancemutater Logger
//go:generate mockgen -package mocks -destination mocks/namestag_mock.go gopkg.in/juju/names.v2 Tag

type InstanceMutaterAPI interface {
	WatchModelMachines() (watcher.StringsWatcher, error)
	Machine(tag names.MachineTag) (instancemutater.MutaterMachine, error)
}

// Logger represents the logging methods called.
type Logger interface {
	Warningf(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
}

// Config represents the configuration required to run a new instance mutater
// worker.
type Config struct {
	Facade InstanceMutaterAPI

	// Logger is the logger for this worker.
	Logger Logger

	Environ environs.Environ

	AgentConfig agent.Config

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
		return errors.NotValidf("nil Facade")
	}
	if config.Environ == nil {
		return errors.NotValidf("nil Environ")
	}
	if config.AgentConfig == nil {
		return errors.NotValidf("nil AgentConfig")
	}
	//if config.Tag == nil {
	//	return errors.NotValidf("nil tag")
	//}
	//if _, ok := config.Tag.(names.MachineTag); !ok {
	//	return errors.NotValidf("tag")
	//}
	return nil
}

// NewWorker returns a worker that keeps track of
// the machines/containers in the state and polls their instance
// for any changes.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	broker, ok := config.Environ.(environs.LXDProfiler)
	if !ok {
		// If we don't have an LXDProfiler broker, there is no need to
		// run this worker.
		config.Logger.Debugf("uninstalling, not an LXD capable broker")
		return nil, dependency.ErrUninstall
	}
	w := &mutaterWorker{
		logger:     config.Logger,
		facade:     config.Facade,
		broker:     broker,
		machineTag: config.AgentConfig.Tag().(names.MachineTag),
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
	facade     InstanceMutaterAPI
}

func (w *mutaterWorker) loop() error {
	watcher, err := w.facade.WatchModelMachines()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}
	return watchMachinesLoop(w, w.logger, watcher)
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
func (w *mutaterWorker) getMachine(tag names.MachineTag) (instancemutater.MutaterMachine, error) {
	m, err := w.facade.Machine(tag)
	return m, err
}

func (w *mutaterWorker) getBroker() environs.LXDProfiler {
	return w.broker
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

// add is part of the lifetimeContext interface.
func (w *mutaterWorker) add(new worker.Worker) error {
	return w.catacomb.Add(new)
}
