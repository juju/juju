// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	coreobjectstore "github.com/juju/juju/core/objectstore"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/worker/trace"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// TrackedObjectStore is a ObjectStore that is also a worker, to ensure the l
// ifecycle of the objectStore is managed.
type TrackedObjectStore interface {
	worker.Worker
	coreobjectstore.ObjectStore
}

// WorkerConfig encapsulates the configuration options for the
// objectStore worker.
type WorkerConfig struct {
	TracerGetter         trace.TracerGetter
	Clock                clock.Clock
	Logger               Logger
	NewObjectStoreWorker ObjectStoreWorkerFunc

	// StatePool is only here for backwards compatibility. Once we have
	// the right abstractions in place, and we have a replacement, we can
	// remove this.
	StatePool StatePool
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.TracerGetter == nil {
		return errors.NotValidf("nil TracerGetter")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.NewObjectStoreWorker == nil {
		return errors.NotValidf("nil NewObjectStoreWorker")
	}
	if c.StatePool == nil {
		return errors.NotValidf("nil StatePool")
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
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &objectStoreWorker{
		internalStates: internalStates,
		cfg:            cfg,
		runner: worker.NewRunner(worker.RunnerParams{
			Clock: cfg.Clock,
			IsFatal: func(err error) bool {
				return false
			},
			RestartDelay: time.Second * 10,
			Logger:       cfg.Logger,
		}),
		objectStoreRequests: make(chan objectStoreRequest),
	}

	if err = catacomb.Invoke(catacomb.Plan{
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

	for {
		select {
		// The following ensures that all objectStoreRequests are serialised and
		// processed in order.
		case req := <-w.objectStoreRequests:
			if err := w.initObjectStore(req.namespace); err != nil {
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
func (w *objectStoreWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *objectStoreWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetObjectStore returns a objectStore for the given namespace.
func (w *objectStoreWorker) GetObjectStore(ctx context.Context, namespace string) (coreobjectstore.ObjectStore, error) {
	// First check if we've already got the objectStore worker already running.
	// If we have, then return out quickly. The objectStoreRunner is the cache,
	// so there is no need to have a in-memory cache here.
	if objectStore, err := w.workerFromCache(namespace); err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, coreobjectstore.ErrObjectStoreDying
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
		return nil, coreobjectstore.ErrObjectStoreDying
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
		return nil, coreobjectstore.ErrObjectStoreDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.runner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return tracked.(coreobjectstore.ObjectStore), nil
}

func (w *objectStoreWorker) workerFromCache(namespace string) (coreobjectstore.ObjectStore, error) {
	// If the worker already exists, return the existing worker early.
	if objectStore, err := w.runner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return objectStore.(coreobjectstore.ObjectStore), nil
	} else if errors.Is(errors.Cause(err), worker.ErrDead) {
		// Handle the case where the DB runner is dead due to this worker dying.
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

func (w *objectStoreWorker) initObjectStore(namespace string) error {
	err := w.runner.StartWorker(namespace, func() (worker.Worker, error) {
		ctx, cancel := w.scopedContext()
		defer cancel()

		tracer, err := w.cfg.TracerGetter.GetTracer(ctx, coretrace.Namespace("objectstore", namespace))
		if err != nil {
			return nil, errors.Trace(err)
		}

		// This is only here until we have a better backing store.
		state, err := w.cfg.StatePool.Get(namespace)
		if err != nil {
			return nil, errors.Trace(err)
		}

		objectStore, err := w.cfg.NewObjectStoreWorker(
			ctx,
			namespace,
			state,
			w.cfg.Logger,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return &tracedWorker{
			TrackedObjectStore: objectStore,
			tracer:             tracer,
		}, nil
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
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

// tracedWorker is a wrapper around a ObjectStore that adds tracing, without
// exposing the underlying ObjectStore.
type tracedWorker struct {
	TrackedObjectStore
	tracer coretrace.Tracer
}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (t *tracedWorker) Get(ctx context.Context, path string) (_ io.ReadCloser, _ int64, err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(coretrace.StringAttr("objectstore.path", path)),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return t.TrackedObjectStore.Get(ctx, path)
}

// Put stores data from reader at path, namespaced to the model.
func (t *tracedWorker) Put(ctx context.Context, path string, r io.Reader, length int64) (err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(
			coretrace.StringAttr("objectstore.path", path),
			coretrace.Int64Attr("objectstore.size", length),
		),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return t.TrackedObjectStore.Put(ctx, path, r, length)
}
