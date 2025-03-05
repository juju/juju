// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	agentengine "github.com/juju/juju/agent/engine"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/http"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/state"
)

// ModelWatcher provides an interface for watching the additiona and
// removal of models.
type ModelWatcher interface {
	WatchModels() state.StringsWatcher
}

// ControllerConfigGetter is an interface that returns the controller config.
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// Controller provides an interface for getting models by UUID,
// and other details needed to pass into the function to start workers for a model.
// Once a model is no longer required, the returned function must
// be called to dispose of the model.
type Controller interface {
	Model(modelUUID string) (Model, func(), error)
}

// Model represents a model.
type Model interface {
	MigrationMode() state.MigrationMode
	Type() state.ModelType
	Name() string
	Owner() names.UserTag
}

// RecordLogger writes logs to backing store.
type RecordLogger interface {
	io.Closer
	// Log writes the given log records to the logger's storage.
	Log([]corelogger.LogRecord) error
}

// MetricSink describes a way to unregister a model metrics collector. This
// ensures that we correctly tidy up after the removal of a model.
type MetricSink = agentengine.MetricSink

// ModelMetrics defines a way to create metrics for a model.
type ModelMetrics interface {
	ForModel(names.ModelTag) MetricSink
}

// GetControllerConfigFunc is a function that returns the controller config,
// from the given service.
type GetControllerConfigFunc func(ctx context.Context, controllerConfigService ControllerConfigService) (controller.Config, error)

// NewModelConfig holds the information required by the NewModelWorkerFunc
// to start the workers for the specified model
type NewModelConfig struct {
	Authority              pki.Authority
	ModelName              string
	ModelOwner             string
	ModelUUID              string
	ModelType              state.ModelType
	ModelMetrics           MetricSink
	LoggerContext          corelogger.LoggerContext
	ControllerConfig       controller.Config
	ProviderServicesGetter ProviderServicesGetter
	DomainServices         services.DomainServices
	HTTPClientGetter       http.HTTPClientGetter
}

// NewModelWorkerFunc should return a worker responsible for running
// all a model's required workers; and for returning nil when there's
// no more model to manage.
type NewModelWorkerFunc func(config NewModelConfig) (worker.Worker, error)

// Config holds the dependencies and configuration necessary to run
// a model worker manager.
type Config struct {
	Authority              pki.Authority
	Logger                 corelogger.Logger
	SystemState            *state.State
	SystemStateService     *domain.StateBase
	ModelMetrics           ModelMetrics
	Mux                    *apiserverhttp.Mux
	Controller             Controller
	NewModelWorker         NewModelWorkerFunc
	ErrorDelay             time.Duration
	LogSinkGetter          corelogger.ModelLogSinkGetter
	ProviderServicesGetter ProviderServicesGetter
	DomainServicesGetter   services.DomainServicesGetter
	GetControllerConfig    GetControllerConfigFunc
	HTTPClientGetter       http.HTTPClientGetter
}

// Validate returns an error if config cannot be expected to drive
// a functional model worker manager.
func (config Config) Validate() error {
	if config.Authority == nil {
		return errors.NotValidf("nil authority")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.SystemState == nil {
		return errors.NotValidf("nil SystemState")
	}
	if config.ModelMetrics == nil {
		return errors.NotValidf("nil ModelMetrics")
	}
	if config.Controller == nil {
		return errors.NotValidf("nil Controller")
	}
	if config.NewModelWorker == nil {
		return errors.NotValidf("nil NewModelWorker")
	}
	if config.LogSinkGetter == nil {
		return errors.NotValidf("nil LogSinkGetter")
	}
	if config.ErrorDelay <= 0 {
		return errors.NotValidf("non-positive ErrorDelay")
	}
	if config.ProviderServicesGetter == nil {
		return errors.NotValidf("nil ProviderServicesGetter")
	}
	if config.DomainServicesGetter == nil {
		return errors.NotValidf("nil DomainServicesGetter")
	}
	if config.GetControllerConfig == nil {
		return errors.NotValidf("nil GetControllerConfig")
	}
	if config.HTTPClientGetter == nil {
		return errors.NotValidf("nil HTTPClientGetter")
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
		runner: worker.NewRunner(worker.RunnerParams{
			IsFatal: neverFatal,
			ShouldRestart: func(err error) bool {
				return !errors.Is(err, database.ErrDBDead)
			},
			MoreImportant: neverImportant,
			RestartDelay:  config.ErrorDelay,
			Logger:        internalworker.WrapLogger(config.Logger),
		}),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &m.catacomb,
		Work: m.loop,
		Init: []worker.Worker{
			m.runner,
		},
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
	systemState := m.config.SystemState
	controllerModelUUID := systemState.ControllerModelUUID()
	domainServicesGetter := m.config.DomainServicesGetter
	domainServices, err := domainServicesGetter.ServicesForModel(context.Background(), model.UUID(controllerModelUUID))
	if err != nil {
		return errors.Trace(err)
	}
	watcher, err := domainServices.Controller().Watch(context.Background())
	if err != nil {
		return errors.Trace(err)
	}

	// watcher = m.config.ModelWatcher.WatchModels()
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
			for _, modelUUID := range uuids {
				if err := m.modelChanged(modelUUID); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (m *modelWorkerManager) modelChanged(modelUUID string) error {
	model, release, err := m.config.Controller.Model(modelUUID)
	if errors.Is(err, errors.NotFound) {
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

	cfg := NewModelConfig{
		Authority:    m.config.Authority,
		ModelName:    model.Name(),
		ModelOwner:   model.Owner().Id(),
		ModelUUID:    modelUUID,
		ModelType:    model.Type(),
		ModelMetrics: m.config.ModelMetrics.ForModel(names.NewModelTag(modelUUID)),
	}
	return errors.Trace(m.ensure(cfg))
}

func (m *modelWorkerManager) ensure(cfg NewModelConfig) error {
	// Creates a new worker func based on the model config.
	starter := m.starter(cfg)
	// If the worker is already running, this will return an AlreadyExists
	// error and the start function will not be called.
	if err := m.runner.StartWorker(cfg.ModelUUID, func() (worker.Worker, error) {
		ctx, cancel := m.scopedContext()
		defer cancel()

		return starter(ctx)
	}); !errors.Is(err, errors.AlreadyExists) {
		return errors.Trace(err)
	}
	return nil
}

func (m *modelWorkerManager) starter(cfg NewModelConfig) func(context.Context) (worker.Worker, error) {
	return func(ctx context.Context) (worker.Worker, error) {
		modelUUID := model.UUID(cfg.ModelUUID)
		modelName := fmt.Sprintf("%q (%s)", fmt.Sprintf("%s-%s", cfg.ModelOwner, cfg.ModelName), modelUUID)
		m.config.Logger.Debugf(ctx, "starting workers for model %s", modelName)

		// Get the provider domain services for the model.
		cfg.ProviderServicesGetter = m.config.ProviderServicesGetter

		var err error
		if cfg.DomainServices, err = m.config.DomainServicesGetter.ServicesForModel(ctx, modelUUID); err != nil {
			return nil, errors.Annotate(err, "unable to get domain services")
		}

		cfg.HTTPClientGetter = m.config.HTTPClientGetter

		// Get the controller config for the model worker so that we correctly
		// handle the case where the controller config changes between model
		// worker restarts.
		controllerConfigService := cfg.DomainServices.ControllerConfig()
		controllerConfig, err := m.config.GetControllerConfig(ctx, controllerConfigService)
		if err != nil {
			return nil, errors.Annotate(err, "unable to get controller config")
		}
		cfg.ControllerConfig = controllerConfig

		// LoggerContext for the model worker, this is then used for all
		// logging.
		cfg.LoggerContext, err = m.config.LogSinkGetter.GetLoggerContext(ctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}

		worker, err := m.config.NewModelWorker(cfg)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot manage model %s", modelName)
		}
		return worker, nil
	}
}

func (m *modelWorkerManager) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(m.catacomb.Context(context.Background()))
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
func (m *modelWorkerManager) Report() map[string]any {
	if m.runner == nil {
		return nil
	}
	return m.runner.Report()
}
