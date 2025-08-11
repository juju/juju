// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	modelerrors "github.com/juju/juju/domain/model/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

// Config describes the dependencies of a Worker.
//
// It's arguable that it should be called WorkerConfig, because of the heavy
// use of model config in this package.
type Config struct {
	TrackerType          TrackerType
	DomainServicesGetter DomainServicesGetter
	GetIAASProvider      GetProviderFunc
	GetCAASProvider      GetProviderFunc
	NewTrackerWorker     NewTrackerWorkerFunc
	NewEphemeralProvider NewEphemeralProviderFunc
	Logger               logger.Logger
	Clock                clock.Clock
}

// Validate returns an error if the config cannot be used to start a Worker.
func (config Config) Validate() error {
	if config.DomainServicesGetter == nil {
		return errors.NotValidf("nil DomainServicesGetter")
	}
	if config.GetIAASProvider == nil {
		return errors.NotValidf("nil GetIAASProvider")
	}
	if config.GetCAASProvider == nil {
		return errors.NotValidf("nil GetCAASProvider")
	}
	if config.NewTrackerWorker == nil {
		return errors.NotValidf("nil NewTrackerWorker")
	}
	if config.NewEphemeralProvider == nil {
		return errors.NotValidf("nil NewEphemeralProvider")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// trackerRequest is used to pass requests for tracker worker
// instances into the worker loop.
type trackerRequest struct {
	namespace string
	done      chan error
}

// providerWorker defines a worker that runs provider tracker workers.
type providerWorker struct {
	internalStates chan string
	catacomb       catacomb.Catacomb
	trackedRunner  *worker.Runner

	config Config

	requests chan trackerRequest
}

// NewWorker creates a new object store worker.
func NewWorker(cfg Config) (worker.Worker, error) {
	return newWorker(cfg, nil)
}

// newWorker creates a new worker to run provider trackers.
func newWorker(config Config, internalStates chan string) (*providerWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "provider-tracker",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return !internalerrors.IsOneOf(
				err,
				modelerrors.NotFound,
				coreerrors.NotFound,
				database.ErrDBDead,
				database.ErrDBNotFound,
			)
		},
		RestartDelay: time.Second * 10,
		Clock:        config.Clock,
		Logger:       internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &providerWorker{
		config:         config,
		trackedRunner:  runner,
		requests:       make(chan trackerRequest),
		internalStates: internalStates,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "provider-tracker",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.trackedRunner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Provider returns the encapsulated provider. It will continue to be updated in
// the background for as long as the Worker continues to run. If the worker
// is not a singular worker, then an error will be returned.
func (w *providerWorker) Provider() (Provider, error) {
	// If we're a singular namespace, we can't get the provider for a model.
	namespace, ok := w.config.TrackerType.SingularNamespace()
	if !ok {
		return nil, errors.NotValidf("provider for non-singular tracker")
	}

	tracker, err := w.workerFromCache(namespace)
	if err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, providertracker.ErrProviderWorkerDying
		}

		return nil, errors.Trace(err)
	} else if tracker != nil {
		return tracker.Provider(), nil
	}

	// If the tracker doesn't exist, then check to see if the worker is dying.
	// Otherwise return an error.
	select {
	case <-w.catacomb.Dying():
		return nil, providertracker.ErrProviderWorkerDying
	default:
		return nil, errors.NotFoundf("provider")
	}
}

// ProviderForModel returns the encapsulated provider for a given model
// namespace. It will continue to be updated in the background for as long as
// the Worker continues to run. If the worker is not a singular worker, then an
// error will be returned.
func (w *providerWorker) ProviderForModel(ctx context.Context, namespace string) (Provider, error) {
	// The controller namespace is the global names and has no models associated
	// with it, so fail early.
	if namespace == database.ControllerNS {
		return nil, errors.NotValidf("provider for controller namespace")
	}
	// If we're a singular namespace, we can't get the provider for a model.
	if _, ok := w.config.TrackerType.SingularNamespace(); ok {
		return nil, errors.NotValidf("provider for model with singular tracker")
	}

	tracker, err := w.workerFromCache(namespace)
	if err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, providertracker.ErrProviderWorkerDying
		}

		return nil, errors.Trace(err)
	} else if tracker != nil {
		return tracker.Provider(), nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := trackerRequest{
		namespace: namespace,
		done:      make(chan error),
	}
	select {
	case w.requests <- req:
	case <-w.catacomb.Dying():
		return nil, providertracker.ErrProviderWorkerDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		// If we know we've got an error, just return that error before
		// attempting to ask the objectStoreRunnerWorker.
		if err != nil {
			return nil, errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return nil, providertracker.ErrProviderWorkerDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.trackedRunner.Worker(namespace, w.catacomb.Dying())
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if tracked == nil {
		return nil, providertracker.ErrProviderNotFound
	}
	return tracked.(*trackerWorker).Provider(), nil
}

// EphemeralProviderFromConfig returns an ephemeral provider for a given
// configuration. The provider is not tracked, instead is created and then
// discarded. Credential invalidation is not enforced during the call to the
// provider. If the credentials change, the provider will have to be recreated.
func (w *providerWorker) EphemeralProviderFromConfig(ctx context.Context, config providertracker.EphemeralProviderConfig) (Provider, error) {
	return w.config.NewEphemeralProvider(ctx, EphemeralConfig{
		ModelType:      config.ModelType,
		ModelConfig:    config.ModelConfig,
		CloudSpec:      config.CloudSpec,
		ControllerUUID: config.ControllerUUID,
		GetProviderForType: getProviderForType(
			w.config.GetIAASProvider,
			w.config.GetCAASProvider,
		),
	})
}

// Kill is part of the worker.Worker interface.
func (w *providerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *providerWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *providerWorker) Report() map[string]any {
	return w.trackedRunner.Report()
}

func (w *providerWorker) loop() (err error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	// If we're a singular namespace, we need to start the worker early.
	if namespace, ok := w.config.TrackerType.SingularNamespace(); ok {
		if err := w.initTrackerWorker(ctx, namespace); err != nil {
			return errors.Trace(err)
		}
	}

	// Report the initial started state.
	w.reportInternalState(stateStarted)

	for {
		select {
		// The following ensures that all requests are serialised and
		// processed in order.
		case req := <-w.requests:
			if err := w.initTrackerWorker(ctx, req.namespace); err != nil {
				select {
				case req.done <- errors.Trace(err):
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				}
				continue
			}

			select {
			case req.done <- nil:
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			}

		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

func (w *providerWorker) workerFromCache(namespace string) (*trackerWorker, error) {
	// If the worker already exists, return the existing worker early.
	if tracker, err := w.trackedRunner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return tracker.(*trackerWorker), nil
	} else if errors.Is(errors.Cause(err), worker.ErrDead) {
		// Handle the case where the runner is dead due to this worker dying.
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		default:
			return nil, errors.Trace(err)
		}
	} else if !errors.Is(errors.Cause(err), errors.NotFound) {
		// If it's not a NotFound error, return the underlying error. We should
		// only start a worker if it doesn't exist yet.
		return nil, errors.Trace(err)
	}
	// We didn't find the worker, so return nil, we'll create it in the next
	// step.
	return nil, nil
}

func (w *providerWorker) initTrackerWorker(ctx context.Context, namespace string) error {
	err := w.trackedRunner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		// Create the tracker worker based on the namespace.
		domainServices := w.config.DomainServicesGetter.ServicesForModel(namespace)

		tracker, err := w.config.NewTrackerWorker(ctx, TrackerConfig{
			ModelService:      domainServices.Model(),
			CloudService:      domainServices.Cloud(),
			ConfigService:     domainServices.Config(),
			CredentialService: domainServices.Credential(),
			GetProviderForType: getProviderForType(
				w.config.GetIAASProvider,
				w.config.GetCAASProvider,
			),
			Logger: w.config.Logger.Child(database.ShortNamespace(namespace)),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return tracker, nil
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

func getProviderForType(getIAASProvider, getCAASProvider GetProviderFunc) func(coremodel.ModelType) (GetProviderFunc, error) {
	return func(modelType coremodel.ModelType) (GetProviderFunc, error) {
		switch modelType {
		case coremodel.IAAS:
			return getIAASProvider, nil
		case coremodel.CAAS:
			return getCAASProvider, nil
		default:
			return nil, errors.Errorf("unknown provider type %q", modelType.String())
		}
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *providerWorker) scopedContext() (context.Context, context.CancelFunc) {
	return w.scopedContextFrom(context.Background())
}

func (w *providerWorker) scopedContextFrom(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	return w.catacomb.Context(ctx), cancel
}

func (w *providerWorker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
