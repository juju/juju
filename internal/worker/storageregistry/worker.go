// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageregistry

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/internal/storage"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// WorkerConfig encapsulates the configuration options for the storage registry
// worker.
type WorkerConfig struct {
	// ProviderFactory is used to get provider instances.
	ProviderFactory          providertracker.ProviderFactory
	NewStorageRegistryWorker StorageRegistryWorkerFunc
	Clock                    clock.Clock
	Logger                   logger.Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.ProviderFactory == nil {
		return errors.NotValidf("nil ProviderFactory")
	}
	if c.NewStorageRegistryWorker == nil {
		return errors.NotValidf("nil NewStorageRegistryWorker")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// storageRegistryRequest is used to pass requests for Storage Registry
// instances into the worker loop.
type storageRegistryRequest struct {
	namespace string
	done      chan error
}

type storageRegistryWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	catacomb       catacomb.Catacomb

	runner *worker.Runner

	storageRegistryRequests chan storageRegistryRequest
}

// NewWorker creates a new object store worker.
func NewWorker(cfg WorkerConfig) (*storageRegistryWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*storageRegistryWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "storage-registry",
		Clock: cfg.Clock,
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return !errors.Is(err, database.ErrDBDead)
		},
		RestartDelay: time.Second * 10,
		Logger:       internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &storageRegistryWorker{
		internalStates:          internalStates,
		cfg:                     cfg,
		runner:                  runner,
		storageRegistryRequests: make(chan storageRegistryRequest),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "storage-registry",
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

func (w *storageRegistryWorker) loop() (err error) {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	for {
		select {
		// The following ensures that all storageRegistryRequests are serialised and
		// processed in order.
		case req := <-w.storageRegistryRequests:
			if err := w.initStorageRegistry(ctx, req.namespace); err != nil {
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

// Kill is part of the worker.Worker interface.
func (w *storageRegistryWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *storageRegistryWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetStorageRegistry returns a storageRegistry for the given namespace.
func (w *storageRegistryWorker) GetStorageRegistry(ctx context.Context, namespace string) (storage.ProviderRegistry, error) {
	// First check if we've already got the storageRegistry worker already running.
	// If we have, then return out quickly. The storageRegistryRunner is the cache,
	// so there is no need to have an in-memory cache here.
	if storageRegistry, err := w.workerFromCache(namespace); err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, corestorage.ErrStorageRegistryDying
		}

		return nil, errors.Trace(err)
	} else if storageRegistry != nil {
		return storageRegistry, nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := storageRegistryRequest{
		namespace: namespace,
		done:      make(chan error),
	}
	select {
	case w.storageRegistryRequests <- req:
	case <-w.catacomb.Dying():
		return nil, corestorage.ErrStorageRegistryDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		// If we know we've got an error, just return that error before
		// attempting to ask the storageRegistryRunnerWorker.
		if err != nil {
			return nil, errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return nil, corestorage.ErrStorageRegistryDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.runner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if tracked == nil {
		return nil, errors.NotFoundf("storageregistry")
	}
	return tracked.(storage.ProviderRegistry), nil
}

func (w *storageRegistryWorker) workerFromCache(namespace string) (storage.ProviderRegistry, error) {
	// If the worker already exists, return the existing worker early.
	if storageRegistry, err := w.runner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return storageRegistry.(storage.ProviderRegistry), nil
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

func (w *storageRegistryWorker) initStorageRegistry(ctx context.Context, namespace string) error {
	runner := providertracker.ProviderRunner[storage.ProviderRegistry](w.cfg.ProviderFactory, namespace)

	err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		storageRegistry, err := runner(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}

		worker, err := w.cfg.NewStorageRegistryWorker(storageRegistry)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return worker, nil
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *storageRegistryWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (w *storageRegistryWorker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
