// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.workers.modelworkermanager")

// ModelWatcher provides an interface for watching the additiona and
// removal of models.
type ModelWatcher interface {
	WatchModels() state.StringsWatcher
}

// ModelGetter provides an interface for getting models by UUID.
// Once a model is no longer required, the returned function must
// be called to dispose of the model.
type ModelGetter interface {
	Model(modelUUID string) (Model, func(), error)
}

// Model represents a model.
type Model interface {
	MigrationMode() state.MigrationMode
	Type() state.ModelType
}

// NewModelWorkerFunc should return a worker responsible for running
// all a model's required workers; and for returning nil when there's
// no more model to manage.
type NewModelWorkerFunc func(modelUUID string, modelType state.ModelType) (worker.Worker, error)

// Config holds the dependencies and configuration necessary to run
// a model worker manager.
type Config struct {
	ModelWatcher   ModelWatcher
	ModelGetter    ModelGetter
	NewModelWorker NewModelWorkerFunc
	ErrorDelay     time.Duration
}

// Validate returns an error if config cannot be expected to drive
// a functional model worker manager.
func (config Config) Validate() error {
	if config.ModelWatcher == nil {
		return errors.NotValidf("nil ModelWatcher")
	}
	if config.ModelGetter == nil {
		return errors.NotValidf("nil ModelGetter")
	}
	if config.NewModelWorker == nil {
		return errors.NotValidf("nil NewModelWorker")
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
		config: config,
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
	runner   *worker.Runner
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
	m.runner = worker.NewRunner(worker.RunnerParams{
		IsFatal:       neverFatal,
		MoreImportant: neverImportant,
		RestartDelay:  m.config.ErrorDelay,
	})
	if err := m.catacomb.Add(m.runner); err != nil {
		return errors.Trace(err)
	}
	watcher := m.config.ModelWatcher.WatchModels()
	if err := m.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	modelChanged := func(modelUUID string) error {
		model, release, err := m.config.ModelGetter.Model(modelUUID)
		if errors.IsNotFound(err) {
			// Model was removed, ignore it.
			return nil
		} else if err != nil {
			return errors.Trace(err)
		}
		defer release()

		if !isModelActive(model) {
			// Ignore this model until it's activated - we
			// never want to run workers for an importing
			// model.
			// https://bugs.launchpad.net/juju/+bug/1646310
			return nil
		}
		return errors.Trace(m.ensure(modelUUID, model.Type()))
	}

	for {
		select {
		case <-m.catacomb.Dying():
			return m.catacomb.ErrDying()
		case uuids, ok := <-watcher.Changes():
			if !ok {
				return errors.New("changes stopped")
			}
			for _, modelUUID := range uuids {
				if err := modelChanged(modelUUID); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (m *modelWorkerManager) ensure(modelUUID string, modelType state.ModelType) error {
	starter := m.starter(modelUUID, modelType)
	if err := m.runner.StartWorker(modelUUID, starter); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *modelWorkerManager) starter(modelUUID string, modelType state.ModelType) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		logger.Debugf("starting workers for model %q", modelUUID)
		worker, err := m.config.NewModelWorker(modelUUID, modelType)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot manage model %q", modelUUID)
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

func isModelActive(m Model) bool {
	return m.MigrationMode() != state.MigrationModeImporting
}

// Report shows up in the dependency engine report.
func (m *modelWorkerManager) Report() map[string]interface{} {
	return m.runner.Report()
}
