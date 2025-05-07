// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"context"
	"fmt"
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
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

// MetricSink describes a way to unregister a model metrics collector. This
// ensures that we correctly tidy up after the removal of a model.
type MetricSink = agentengine.MetricSink

// ModelService provides access to the model services required by the
// apiserver.
type ModelService interface {
	// WatchActivatedModels returns a watcher that emits an event containing the model UUID
	// when a model becomes activated or an activated model receives an update.
	WatchActivatedModels(ctx context.Context) (watcher.StringsWatcher, error)

	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid model.UUID) (model.Model, error)
}

// ModelMetrics defines a way to create metrics for a model.
type ModelMetrics interface {
	ForModel(names.ModelTag) MetricSink
}

// GetControllerConfigFunc is a function that returns the controller config,
// from the given service.
type GetControllerConfigFunc func(ctx context.Context, domainServices services.DomainServices) (controller.Config, error)

// NewModelConfig holds the information required by the NewModelWorkerFunc
// to start the workers for the specified model
type NewModelConfig struct {
	Authority              pki.Authority
	ModelName              string
	ModelOwner             string
	ModelUUID              string
	ModelType              model.ModelType
	ModelMetrics           MetricSink
	LoggerContext          corelogger.LoggerContext
	ControllerConfig       controller.Config
	ProviderServicesGetter ProviderServicesGetter
	DomainServices         services.DomainServices
	LeaseManager           lease.Manager
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
	ModelMetrics           ModelMetrics
	Mux                    *apiserverhttp.Mux
	NewModelWorker         NewModelWorkerFunc
	ErrorDelay             time.Duration
	LogSinkGetter          corelogger.ModelLogSinkGetter
	ProviderServicesGetter ProviderServicesGetter
	DomainServicesGetter   services.DomainServicesGetter
	ModelService           ModelService
	GetControllerConfig    GetControllerConfigFunc
	LeaseManager           lease.Manager
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
	if config.ModelService == nil {
		return errors.NotValidf("nil ModelService")
	}
	if config.ModelMetrics == nil {
		return errors.NotValidf("nil ModelMetrics")
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
	if config.LeaseManager == nil {
		return errors.NotValidf("nil LeaseManager")
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

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:    "model-worker-manager",
		IsFatal: neverFatal,
		ShouldRestart: func(err error) bool {
			return !errors.Is(err, database.ErrDBDead)
		},
		MoreImportant: neverImportant,
		RestartDelay:  config.ErrorDelay,
		Logger:        internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	m := &modelWorkerManager{
		config: config,
		runner: runner,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &m.catacomb,
		Work: m.loop,
		Init: []worker.Worker{
			m.runner,
		},
	}); err != nil {
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
	ctx, cancel := m.scopedContext()
	defer cancel()
	watcher, err := m.config.ModelService.WatchActivatedModels(ctx)
	if err != nil {
		return errors.Trace(err)
	}

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
				if err := m.modelChanged(ctx, modelUUID); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (m *modelWorkerManager) modelChanged(ctx context.Context, modelUUID string) error {
	model, err := m.config.ModelService.Model(ctx, model.UUID(modelUUID))

	// If the model is not found, it means two things, either it was removed or
	// more likely it was never activated. In either case, we don't need to
	// start a worker for it.
	if errors.Is(err, modelerrors.NotFound) {
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

	cfg := NewModelConfig{
		Authority:    m.config.Authority,
		ModelName:    model.Name,
		ModelOwner:   model.OwnerName.Name(),
		ModelUUID:    modelUUID,
		ModelType:    model.ModelType,
		ModelMetrics: m.config.ModelMetrics.ForModel(names.NewModelTag(modelUUID)),
	}

	// Creates a new worker func based on the model config.
	newWorker, err := m.newWorkerFuncFromConfig(ctx, cfg)
	if err != nil {
		return errors.Trace(err)
	}

	// If the worker is already running, this will return an AlreadyExists
	// error and the start function will not be called.
	if err := m.runner.StartWorker(ctx, modelUUID, func(ctx context.Context) (worker.Worker, error) {
		return newWorker(ctx)
	}); !errors.Is(err, errors.AlreadyExists) {
		return errors.Trace(err)
	}

	return nil
}

func (m *modelWorkerManager) newWorkerFuncFromConfig(ctx context.Context, cfg NewModelConfig) (func(context.Context) (worker.Worker, error), error) {
	modelUUID := model.UUID(cfg.ModelUUID)
	modelName := fmt.Sprintf("%q (%s)", fmt.Sprintf("%s-%s", cfg.ModelOwner, cfg.ModelName), modelUUID)

	// Get the provider domain services for the model.
	cfg.ProviderServicesGetter = m.config.ProviderServicesGetter

	cfg.LeaseManager = m.config.LeaseManager
	cfg.HTTPClientGetter = m.config.HTTPClientGetter

	// We don't want to get this in the start worker function because it
	// won't change. Hammering the domainservices getter to get the services
	// if the model worker is constantly restarting isn't helping anyone.
	// Especially if the model is in a bad state.
	domainServices, err := m.config.DomainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Annotate(err, "unable to get domain services")
	}
	cfg.DomainServices = domainServices

	// LoggerContext for the model worker, this is then used for all
	// logging.
	cfg.LoggerContext, err = m.config.LogSinkGetter.GetLoggerContext(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx context.Context) (worker.Worker, error) {
		m.config.Logger.Debugf(ctx, "starting workers for model %s", modelName)

		// Get the controller config for the model worker so that we correctly
		// handle the case where the controller config changes between model
		// worker restarts.
		controllerConfig, err := m.config.GetControllerConfig(ctx, domainServices)
		if err != nil {
			return nil, errors.Annotate(err, "unable to get controller config")
		}
		cfg.ControllerConfig = controllerConfig

		worker, err := m.config.NewModelWorker(cfg)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot manage model %s", modelName)
		}
		return worker, nil
	}, nil
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

// Report shows up in the dependency engine report.
func (m *modelWorkerManager) Report() map[string]any {
	if m.runner == nil {
		return nil
	}
	return m.runner.Report()
}
