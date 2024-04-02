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
)

const (
	// ErrProviderWorkerDying is returned when the provider worker is dying.
	ErrProviderWorkerDying = errors.ConstError("provider worker is dying")
)

// Config describes the dependencies of a Worker.
//
// It's arguable that it should be called WorkerConfig, because of the heavy
// use of model config in this package.
type Config[T Provider] struct {
	TrackerType          TrackerType
	ServiceFactoryGetter ServiceFactoryGetter
	GetProvider          GetProviderFunc[T]
	NewTrackerWorker     NewTrackerWorkerFunc[T]
	Logger               Logger
	Clock                clock.Clock
}

// Validate returns an error if the config cannot be used to start a Worker.
func (config Config[T]) Validate() error {
	if config.ServiceFactoryGetter == nil {
		return errors.NotValidf("nil ServiceFactoryGetter")
	}
	if config.GetProvider == nil {
		return errors.NotValidf("nil GetProvider")
	}
	if config.NewTrackerWorker == nil {
		return errors.NotValidf("nil NewTrackerWorker")
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
type providerWorker[T Provider] struct {
	internalStates chan string
	catacomb       catacomb.Catacomb
	runner         *worker.Runner

	config Config[T]

	requests chan trackerRequest
}

// NewWorker creates a new object store worker.
func NewWorker[T Provider](cfg Config[T]) (worker.Worker, error) {
	return newWorker(cfg, nil)
}

// newWorker creates a new worker to run provider trackers.
func newWorker[T Provider](config Config[T], internalStates chan string) (*providerWorker[T], error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &providerWorker[T]{
		config: config,
		runner: worker.NewRunner(worker.RunnerParams{
			IsFatal: func(err error) bool {
				return false
			},
			RestartDelay: time.Second * 10,
			Clock:        config.Clock,
			Logger:       config.Logger,
		}),
		requests:       make(chan trackerRequest),
		internalStates: internalStates,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Provider returns the encapsulated Environ. It will continue to be updated in
// the background for as long as the Worker continues to run. If the worker
// is not a singular worker, then an error will be returned.
func (w *providerWorker[T]) Provider() (res T, err error) {
	// If we're a singular namespace, we can't get the provider for a model.
	namespace, ok := w.config.TrackerType.SingularNamespace()
	if !ok {
		return res, errors.NotValidf("provider for non-singular tracker")
	}

	tracker, err := w.workerFromCache(namespace)
	if err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return res, ErrProviderWorkerDying
		}

		return res, errors.Trace(err)
	} else if tracker != nil {
		return tracker.Provider(), nil
	}

	// If the tracker doesn't exist, then check to see if the worker is dying.
	// Otherwise return an error.
	select {
	case <-w.catacomb.Dying():
		return res, ErrProviderWorkerDying
	default:
		return res, errors.NotFoundf("provider")
	}
}

// ProviderForModel returns the encapsulated Environ for a given model
// namespace. It will continue to be updated in the background for as long as
// the Worker continues to run. If the worker is not a singular worker, then an
// error will be returned.
func (w *providerWorker[T]) ProviderForModel(ctx context.Context, namespace string) (res T, err error) {
	// The controller namespace is the global names and has no models associated
	// with it, so fail early.
	if namespace == database.ControllerNS {
		return res, errors.NotValidf("provider for controller namespace")
	}
	// If we're a singular namespace, we can't get the provider for a model.
	if _, ok := w.config.TrackerType.SingularNamespace(); ok {
		return res, errors.NotValidf("provider for model with singular tracker")
	}

	tracker, err := w.workerFromCache(namespace)
	if err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return res, ErrProviderWorkerDying
		}

		return res, errors.Trace(err)
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
		return res, ErrProviderWorkerDying
	case <-ctx.Done():
		return res, errors.Trace(ctx.Err())
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		// If we know we've got an error, just return that error before
		// attempting to ask the objectStoreRunnerWorker.
		if err != nil {
			return res, errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return res, ErrProviderWorkerDying
	case <-ctx.Done():
		return res, errors.Trace(ctx.Err())
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.runner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return res, errors.Trace(err)
	}
	if tracked == nil {
		return res, errors.NotFoundf("provider")
	}
	return tracked.(*trackerWorker[T]).Provider(), nil
}

// Kill is part of the worker.Worker interface.
func (w *providerWorker[T]) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *providerWorker[T]) Wait() error {
	return w.catacomb.Wait()
}

func (w *providerWorker[T]) loop() (err error) {
	// If we're a singular namespace, we need to start the worker early.
	if namespace, ok := w.config.TrackerType.SingularNamespace(); ok {
		if err := w.initTrackerWorker(namespace); err != nil {
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
			if err := w.initTrackerWorker(req.namespace); err != nil {
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

func (w *providerWorker[T]) workerFromCache(namespace string) (*trackerWorker[T], error) {
	// If the worker already exists, return the existing worker early.
	if tracker, err := w.runner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return tracker.(*trackerWorker[T]), nil
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

func (w *providerWorker[T]) initTrackerWorker(namespace string) error {
	err := w.runner.StartWorker(namespace, func() (worker.Worker, error) {
		ctx, cancel := w.scopedContext()
		defer cancel()

		// Create the tracker worker based on the namespace.
		serviceFactory := w.config.ServiceFactoryGetter.FactoryForModel(namespace)

		tracker, err := w.config.NewTrackerWorker(ctx, TrackerConfig[T]{
			ModelService:      serviceFactory.Model(),
			CloudService:      serviceFactory.Cloud(),
			ConfigService:     serviceFactory.Config(),
			CredentialService: serviceFactory.Credential(),
			GetProvider:       w.config.GetProvider,
			Logger:            w.config.Logger,
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

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *providerWorker[T]) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (w *providerWorker[T]) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
