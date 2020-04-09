// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"fmt"
	"time"

	"github.com/juju/loggo"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/state"
)

// ModelWatcher provides an interface for watching the additiona and
// removal of models.
type ModelWatcher interface {
	WatchModels() state.StringsWatcher
}

// Controller provides an interface for getting models by UUID,
// and other details needed to pass into the function to start workers for a model.
// Once a model is no longer required, the returned function must
// be called to dispose of the model.
type Controller interface {
	Config() (controller.Config, error)
	Model(modelUUID string) (Model, func(), error)
	DBLogger(modelUUID string) (DBLogger, error)
}

// Model represents a model.
type Model interface {
	MigrationMode() state.MigrationMode
	Type() state.ModelType
	Name() string
	Owner() names.UserTag
}

// DBLogger writes into the log collections.
type DBLogger interface {
	// Log writes the given log records to the logger's storage.
	Log([]state.LogRecord) error
	Close()
}

// ModelLogger is a database backed loggo Writer.
type ModelLogger interface {
	loggo.Writer
	Close() error
}

// NewModelConfig holds the information required by the NewModelWorkerFunc
// to start the workers for the specified model
type NewModelConfig struct {
	Authority        pki.Authority
	ModelName        string // Use a fully qualified name "<namespace>-<name>"
	ModelUUID        string
	ModelType        state.ModelType
	ModelLogger      ModelLogger
	Mux              *apiserverhttp.Mux
	ControllerConfig controller.Config
}

// NewModelWorkerFunc should return a worker responsible for running
// all a model's required workers; and for returning nil when there's
// no more model to manage.
type NewModelWorkerFunc func(config NewModelConfig) (worker.Worker, error)

// Config holds the dependencies and configuration necessary to run
// a model worker manager.
type Config struct {
	Authority      pki.Authority
	Clock          clock.Clock
	Logger         Logger
	MachineID      string
	ModelWatcher   ModelWatcher
	Mux            *apiserverhttp.Mux
	Controller     Controller
	NewModelWorker NewModelWorkerFunc
	ErrorDelay     time.Duration
}

// Validate returns an error if config cannot be expected to drive
// a functional model worker manager.
func (config Config) Validate() error {
	if config.Authority == nil {
		return errors.NotValidf("nil authority")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.MachineID == "" {
		return errors.NotValidf("empty MachineID")
	}
	if config.ModelWatcher == nil {
		return errors.NotValidf("nil ModelWatcher")
	}
	if config.Controller == nil {
		return errors.NotValidf("nil Controller")
	}
	if config.NewModelWorker == nil {
		return errors.NotValidf("nil NewModelWorker")
	}
	if config.ErrorDelay <= 0 {
		return errors.NotValidf("non-positive ErrorDelay")
	}
	return nil
}

// New starts a new model worker manager.
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
	controllerConfig, err := m.config.Controller.Config()
	if err != nil {
		return errors.Annotate(err, "unable to get controller config")
	}
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
		model, release, err := m.config.Controller.Model(modelUUID)
		if errors.IsNotFound(err) {
			// Model was removed, ignore it.
			// The reason we ignore it here is that one of the embedded
			// workers is also responding to the model life changes and
			// when it returns a NotFound error, which is determined as a
			// fatal error for the model worker engine. This causes it to be
			// removed from the runner above. However since the runner itself
			// has neverFatal as an error handler, the runner itself doesn't
			// propagate the error.
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
		cfg := NewModelConfig{
			Authority:        m.config.Authority,
			ModelName:        fmt.Sprintf("%s-%s", model.Owner().Id(), model.Name()),
			ModelUUID:        modelUUID,
			ModelType:        model.Type(),
			Mux:              m.config.Mux,
			ControllerConfig: controllerConfig,
		}
		return errors.Trace(m.ensure(cfg))
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

func (m *modelWorkerManager) ensure(cfg NewModelConfig) error {
	starter := m.starter(cfg)
	if err := m.runner.StartWorker(cfg.ModelUUID, starter); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *modelWorkerManager) starter(cfg NewModelConfig) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		modelUUID := cfg.ModelUUID
		modelName := fmt.Sprintf("%q (%s)", cfg.ModelName, cfg.ModelUUID)
		m.config.Logger.Debugf("starting workers for model %s", modelName)
		dbLogger, err := m.config.Controller.DBLogger(modelUUID)
		if err != nil {
			return nil, errors.Annotatef(err, "unable to create db logger for %s", modelName)
		}
		cfg.ModelLogger = newModelLogger(
			"controller-"+m.config.MachineID,
			modelUUID,
			dbLogger,
			m.config.Clock,
			m.config.Logger,
		)
		worker, err := m.config.NewModelWorker(cfg)
		if err != nil {
			cfg.ModelLogger.Close()
			return nil, errors.Annotatef(err, "cannot manage model %s", modelName)
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
