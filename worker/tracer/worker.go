// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
)

type TrackedTracer interface {
	worker.Worker
	Tracer
}

// WorkerConfig encapsulates the configuration options for the
// tracer worker.
type WorkerConfig struct {
	Clock           clock.Clock
	Logger          Logger
	NewTracerWorker TracerWorkerFunc
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.NewTracerWorker == nil {
		return errors.NotValidf("nil NewTracerWorker")
	}
	return nil
}

// traceRequest is used to pass requests for Tracer
// instances into the worker loop.
type traceRequest struct {
	namespace string
	done      chan error
}

type tracerWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	tracerRunner *worker.Runner

	// tracerRequests is used to synchronise GetTracer
	// requests into this worker's event loop.
	tracerRequests chan traceRequest
}

// NewWorker creates a new tracer worker.
func NewWorker(cfg WorkerConfig) (*tracerWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &tracerWorker{
		cfg: cfg,
		tracerRunner: worker.NewRunner(worker.RunnerParams{
			Clock: cfg.Clock,
			IsFatal: func(err error) bool {
				return false
			},
			RestartDelay: time.Second * 10,
			Logger:       cfg.Logger,
		}),
		tracerRequests: make(chan traceRequest),
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.tracerRunner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *tracerWorker) loop() (err error) {
	for {
		select {
		// The following ensures that all tracerRequests are serialised and
		// processed in order.
		case req := <-w.tracerRequests:
			if err := w.initTracer(req.namespace); err != nil {
				req.done <- errors.Trace(err)
				continue
			}

			req.done <- nil

		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *tracerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *tracerWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetTracer returns a tracer for the given namespace.
func (w *tracerWorker) GetTracer(namespace string) (Tracer, error) {
	// First check if we've already got the tracer worker already running. If
	// we have, then return out quickly. The tracerRunner is the cache, so there
	// is no need to have a in-memory cache here.
	if tracer, err := w.workerFromCache(namespace); err != nil {
		return nil, errors.Trace(err)
	} else if tracer != nil {
		return tracer, nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := traceRequest{
		namespace: namespace,
		done:      make(chan error),
	}
	select {
	case w.tracerRequests <- req:
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		// If we know we've got an error, just return that error before
		// attempting to ask the tracerRunnerWorker.
		if err != nil {
			return nil, errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.tracerRunner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return tracked.(Tracer), nil
}

func (w *tracerWorker) workerFromCache(namespace string) (Tracer, error) {
	// If the worker already exists, return the existing worker early.
	if tracer, err := w.tracerRunner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return tracer.(Tracer), nil
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

func (w *tracerWorker) initTracer(namespace string) error {
	err := w.tracerRunner.StartWorker(namespace, func() (worker.Worker, error) {
		ctx, cancel := w.scopedContext()
		defer cancel()

		return w.cfg.NewTracerWorker(ctx, namespace, w.cfg.Logger)
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *tracerWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}
