// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"go.opentelemetry.io/otel"
	"gopkg.in/tomb.v2"

	coretracing "github.com/juju/juju/core/tracing"
)

type TrackedTracer interface {
	worker.Worker
	coretracing.Tracer
}

// WorkerConfig encapsulates the configuration options for the
// tracer worker.
type WorkerConfig struct {
	Clock           clock.Clock
	Logger          Logger
	NewTracerWorker TracerWorkerFunc

	Endpoint           string
	InsecureSkipVerify bool
	StackTracesEnabled bool
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
	// If we are enabled, then we require an endpoint.
	if c.Endpoint == "" {
		return errors.NotValidf("empty Endpoint")
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
	// For some reason, unbeknownst to me, the otel sdk has a global logger
	// that is registered on init. Considering this is a new package, I'm not
	// sure why they decided to do it like this.
	otel.SetLogger(logr.New(&loggoSink{Logger: w.cfg.Logger}))

	for {
		select {
		// The following ensures that all tracerRequests are serialised and
		// processed in order.
		case req := <-w.tracerRequests:
			if err := w.initTracer(req.namespace); err != nil {
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
func (w *tracerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *tracerWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetTracer returns a tracer for the given namespace.
func (w *tracerWorker) GetTracer(namespace string) (coretracing.Tracer, error) {
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

	return tracked.(coretracing.Tracer), nil
}

func (w *tracerWorker) workerFromCache(namespace string) (coretracing.Tracer, error) {
	// If the worker already exists, return the existing worker early.
	if tracer, err := w.tracerRunner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return tracer.(coretracing.Tracer), nil
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

		return w.cfg.NewTracerWorker(
			ctx,
			namespace,
			w.cfg.Endpoint,
			w.cfg.InsecureSkipVerify,
			w.cfg.StackTracesEnabled,
			w.cfg.Logger,
		)
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

// NoopWorker ensures that we get a functioning tracer even if we're not using
// it.
type noopWorker struct {
	tomb tomb.Tomb

	tracer coretracing.Tracer
}

// NewNoopWorker worker creates a worker that doesn't perform any new work on
// the context. Though it will manage the lifecycle of the worker.
func NewNoopWorker() *noopWorker {
	// Set this up, so we only ever hand out a singular tracer and span per
	// worker.
	w := &noopWorker{
		tracer: noopTracer{
			span: noopSpan{},
		},
	}

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})

	return w
}

// GetTracer returns a tracer for the namespace.
// The noopWorker will return a stub tracer in this case.
func (w *noopWorker) GetTracer(namespace string) (coretracing.Tracer, error) {
	return w.tracer, nil
}

// Kill is part of the worker.Worker interface.
func (w *noopWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *noopWorker) Wait() error {
	return w.tomb.Wait()
}

type noopTracer struct {
	span coretracing.Span
}

func (t noopTracer) Start(ctx context.Context, name string, opts ...coretracing.Option) (context.Context, coretracing.Span) {
	return ctx, t.span
}

type noopSpan struct{}

func (noopSpan) AddEvent(string, ...coretracing.Attribute)   {}
func (noopSpan) RecordError(error, ...coretracing.Attribute) {}
func (noopSpan) End(...coretracing.Attribute)                {}

type loggoSink struct {
	Logger        Logger
	name          string
	keysAndValues []any
}

// Init receives optional information about the logr library for LogSink
// implementations that need it.
func (s *loggoSink) Init(info logr.RuntimeInfo) {}

// Enabled tests whether this LogSink is enabled at the specified V-level.
// For example, commandline flags might be used to set the logging
// verbosity and disable some info logs.
func (s *loggoSink) Enabled(level int) bool {
	return s.Logger.IsLevelEnabled(level)
}

// Info logs a non-error message with the given key/value pairs as context.
// The level argument is provided for optional logging.  This method will
// only be called when Enabled(level) is true. See Logger.Info for more
// details.
func (s *loggoSink) Info(level int, msg string, keysAndValues ...any) {
	format, args := s.formatKeysAndValues([]any{level, msg}, keysAndValues)
	s.Logger.Infof("%d: %s"+format, args...)
}

// Error logs an error, with the given message and key/value pairs as
// context.  See Logger.Error for more details.
func (s *loggoSink) Error(err error, msg string, keysAndValues ...any) {
	format, args := s.formatKeysAndValues([]any{msg, err}, keysAndValues)
	s.Logger.Errorf("%s: %v"+format, args...)
}

// WithValues returns a new LogSink with additional key/value pairs.  See
// Logger.WithValues for more details.
func (s *loggoSink) WithValues(keysAndValues ...any) logr.LogSink {
	return &loggoSink{
		Logger:        s.Logger,
		name:          s.name,
		keysAndValues: append(s.keysAndValues, keysAndValues...),
	}
}

// WithName returns a new LogSink with the specified name appended.  See
// Logger.WithName for more details.
func (s *loggoSink) WithName(name string) logr.LogSink {
	return &loggoSink{
		Logger:        s.Logger,
		name:          name,
		keysAndValues: s.keysAndValues,
	}
}

func (s *loggoSink) formatKeysAndValues(init []any, keysAndValues []any) (string, []any) {
	var exprs []string

	kv := append(s.keysAndValues, keysAndValues...)
	args := init

	for _, v := range kv {
		exprs = append(exprs, "%v")
		args = append(args, v)
	}
	if len(exprs) == 0 {
		return "", nil
	}

	format := ": " + strings.Join(exprs, " ")
	return format, args
}
