// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

// Backend defines the State functionality used by the manager worker.
type Backend interface {
	WatchModels() state.StringsWatcher
}

// NewWorkerFunc should return a worker responsible for running
// all a model's required workers; and for returning nil when
// there's no more model to manage.
type NewWorkerFunc func(modelUUID string) (worker.Worker, error)

// Config holds the dependencies and configuration necessary to run
// a model worker manager.
type Config struct {
	Backend    Backend
	NewWorker  NewWorkerFunc
	ErrorDelay time.Duration
}

// Validate returns an error if config cannot be expected to drive
// a functional model worker manager.
func (config Config) Validate() error {
	if config.Backend == nil {
		return errors.NotValidf("nil Backend")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.ErrorDelay <= 0 {
		return errors.NotValidf("non-positive ErrorDelay")
	}
	return nil
}

func New(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	m := &modelWorkerManager{
		config:  config,
		started: set.NewStrings(),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &m.catacomb,
		Work: m.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}

type modelWorkerManager struct {
	catacomb catacomb.Catacomb
	config   Config
	runner   worker.Runner
	started  set.Strings
}

// Kill satisfies the Worker interface.
func (m *modelWorkerManager) Kill() {
	m.catacomb.Kill(nil)
}

// Wait satisfies the Worker interface.
func (m *modelWorkerManager) Wait() error {
	return m.catacomb.Wait()
}

func (m *modelWorkerManager) loop() error {
	m.runner = worker.NewRunner(
		neverFatal, neverImportant, m.config.ErrorDelay,
	)
	if err := m.catacomb.Add(m.runner); err != nil {
		return errors.Trace(err)
	}
	watcher := m.config.Backend.WatchModels()
	if err := m.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-m.catacomb.Dying():
			return m.catacomb.ErrDying()
		case uuids, ok := <-watcher.Changes():
			if !ok {
				return errors.New("changes stopped")
			}
			for _, uuid := range uuids {
				if err := m.ensure(uuid); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (m *modelWorkerManager) ensure(uuid string) error {
	if m.started.Contains(uuid) {
		// A second StartWorker for a given ID is mostly
		// harmless -- it will probably be ignored, but
		// might work if the previous worker has already
		// stopped without error. Neither situation will
		// hurt us directly, but we prefer to eliminate
		// the messy uncertainty.
		return nil
	}
	starter := m.starter(uuid)
	if err := m.runner.StartWorker(uuid, starter); err != nil {
		return errors.Trace(err)
	}
	m.started.Add(uuid)
	return nil
}

func (m *modelWorkerManager) starter(uuid string) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		worker, err := m.config.NewWorker(uuid)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot manage model %q", uuid)
		}
		return worker, nil
	}
}

func neverFatal(error) bool {
	return false
}

func neverImportant(error, error) bool {
	return false
}
