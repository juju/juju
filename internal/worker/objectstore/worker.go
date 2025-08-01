// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	coretrace "github.com/juju/juju/core/trace"
	modelerrors "github.com/juju/juju/domain/model/errors"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/apiremotecaller"
	"github.com/juju/juju/internal/worker/trace"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// TrackedObjectStore is a ObjectStore that is also a worker, to ensure the
// lifecycle of the objectStore is managed.
type TrackedObjectStore interface {
	worker.Worker
	objectstore.ObjectStore
	objectstore.ObjectStoreRemover
	Report() map[string]any
}

// WorkerConfig encapsulates the configuration options for the
// objectStore worker.
type WorkerConfig struct {
	TracerGetter               trace.TracerGetter
	RootDir                    string
	RootBucket                 string
	Clock                      clock.Clock
	Logger                     logger.Logger
	S3Client                   objectstore.Client
	APIRemoteCaller            apiremotecaller.APIRemoteCallers
	NewObjectStoreWorker       internalobjectstore.ObjectStoreWorkerFunc
	ObjectStoreType            objectstore.BackendType
	ControllerMetadataService  MetadataService
	ModelMetadataServiceGetter MetadataServiceGetter
	ModelServiceGetter         ModelServiceGetter
	ModelClaimGetter           ModelClaimGetter
	AllowDraining              bool
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.TracerGetter == nil {
		return errors.NotValidf("nil TracerGetter")
	}
	if c.RootDir == "" {
		return errors.NotValidf("empty RootDir")
	}
	if c.RootBucket == "" {
		return errors.NotValidf("empty RootBucket")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.S3Client == nil {
		return errors.NotValidf("nil S3Client")
	}
	if c.APIRemoteCaller == nil {
		return errors.NotValidf("nil APIRemoteCaller")
	}
	if c.NewObjectStoreWorker == nil {
		return errors.NotValidf("nil NewObjectStoreWorker")
	}
	if c.ControllerMetadataService == nil {
		return errors.NotValidf("nil ControllerMetadataService")
	}
	if c.ModelMetadataServiceGetter == nil {
		return errors.NotValidf("nil ModelMetadataServiceGetter")
	}
	if c.ModelClaimGetter == nil {
		return errors.NotValidf("nil ModelClaimGetter")
	}
	return nil
}

// objectStoreRequest is used to pass requests for ObjectStore
// instances into the worker loop.
type objectStoreRequest struct {
	namespace string
	done      chan error
}

type objectStoreWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	catacomb       catacomb.Catacomb

	runner *worker.Runner

	objectStoreRequests chan objectStoreRequest
}

// NewWorker creates a new object store worker.
func NewWorker(cfg WorkerConfig) (*objectStoreWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*objectStoreWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "object-store",
		Clock: cfg.Clock,
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			if errors.Is(err, modelerrors.NotFound) || errors.Is(err, database.ErrDBDead) {
				return false
			}
			return true
		},
		RestartDelay: time.Second * 10,
		Logger:       internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &objectStoreWorker{
		internalStates:      internalStates,
		cfg:                 cfg,
		runner:              runner,
		objectStoreRequests: make(chan objectStoreRequest),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "object-store",
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

func (w *objectStoreWorker) loop() (err error) {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		// The following ensures that all objectStoreRequests are serialised and
		// processed in order.
		case req := <-w.objectStoreRequests:
			err := w.initObjectStore(ctx, req.namespace)

			select {
			case req.done <- err:
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *objectStoreWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *objectStoreWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetObjectStore returns a objectStore for the given namespace.
func (w *objectStoreWorker) GetObjectStore(ctx context.Context, namespace string) (objectstore.ObjectStore, error) {
	// First check if we've already got the objectStore worker already running.
	// If we have, then return out quickly. The objectStoreRunner is the cache,
	// so there is no need to have an in-memory cache here.
	if objectStore, err := w.workerFromCache(namespace); err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, objectstore.ErrObjectStoreDying
		}

		return nil, errors.Trace(err)
	} else if objectStore != nil {
		return objectStore, nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := objectStoreRequest{
		namespace: namespace,
		done:      make(chan error),
	}
	select {
	case w.objectStoreRequests <- req:
	case <-w.catacomb.Dying():
		return nil, objectstore.ErrObjectStoreDying
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
		return nil, objectstore.ErrObjectStoreDying
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
		return nil, errors.NotFoundf("objectstore")
	}
	return tracked.(objectstore.ObjectStore), nil
}

func (w *objectStoreWorker) workerFromCache(namespace string) (objectstore.ObjectStore, error) {
	// If the worker already exists, return the existing worker early.
	if objectStore, err := w.runner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return objectStore.(objectstore.ObjectStore), nil
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

func (w *objectStoreWorker) initObjectStore(ctx context.Context, namespace string) error {
	err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		tracer, err := w.cfg.TracerGetter.GetTracer(ctx, coretrace.Namespace("objectstore", namespace))
		if err != nil {
			return nil, errors.Annotatef(err, "getting tracer for namespace %q", namespace)
		}

		modelUUID := model.UUID(namespace)

		// Grab the claimer for the model.
		claimer, err := w.cfg.ModelClaimGetter.ForModelUUID(modelUUID)
		if err != nil {
			return nil, errors.Annotatef(err, "getting model claimer for model %q", modelUUID)
		}

		var metadataService MetadataService
		if namespace == database.ControllerNS {
			metadataService = w.cfg.ControllerMetadataService
		} else {
			metadataService = w.cfg.ModelMetadataServiceGetter.ForModelUUID(modelUUID)
		}

		objectStore, err := w.cfg.NewObjectStoreWorker(
			ctx,
			internalobjectstore.BackendTypeOrDefault(w.cfg.ObjectStoreType),
			namespace,
			internalobjectstore.WithRootDir(w.cfg.RootDir),
			internalobjectstore.WithRootBucket(w.cfg.RootBucket),
			internalobjectstore.WithS3Client(w.cfg.S3Client),
			internalobjectstore.WithAPIRemoveCallers(w.cfg.APIRemoteCaller),
			internalobjectstore.WithMetadataService(metadataService),
			internalobjectstore.WithClaimer(claimer),
			internalobjectstore.WithLogger(w.cfg.Logger),
			internalobjectstore.WithAllowDraining(w.cfg.AllowDraining),
		)
		if err != nil {
			return nil, errors.Annotatef(err, "creating object store for namespace %q", namespace)
		}

		if namespace == database.ControllerNS {
			// If we're in the controller namespace, then agents should only
			// be using this. We don't need to track the model service.
			return newControllerWorker(
				objectStore,
				tracer,
			)
		}

		modelServices := w.cfg.ModelServiceGetter.ForModelUUID(modelUUID)
		modelService := modelServices.ModelService()
		return newTrackerWorker(
			modelUUID,
			modelService,
			objectStore,
			tracer,
			w.cfg.Logger,
		)
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

// Report returns a map of internal state for the worker.
func (w *objectStoreWorker) Report() map[string]any {
	return w.runner.Report()
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *objectStoreWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (w *objectStoreWorker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
