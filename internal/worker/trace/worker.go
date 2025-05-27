// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"go.opentelemetry.io/otel"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// TrackedTracer is a Tracer that is also a worker, to ensure the lifecycle of
// the tracer is managed.
type TrackedTracer interface {
	worker.Worker
	coretrace.Tracer
}

// WorkerConfig encapsulates the configuration options for the
// tracer worker.
type WorkerConfig struct {
	Clock           clock.Clock
	Logger          logger.Logger
	NewTracerWorker TracerWorkerFunc

	Tag  names.Tag
	Kind coretrace.Kind

	Endpoint              string
	InsecureSkipVerify    bool
	StackTracesEnabled    bool
	SampleRatio           float64
	TailSamplingThreshold time.Duration
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
	if c.Tag == nil {
		return errors.NotValidf("nil Tag")
	}
	if c.Kind == "" {
		return errors.NotValidf("nil or empty Kind")
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
	namespace coretrace.TaggedTracerNamespace
	done      chan error
}

type tracerWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	catacomb       catacomb.Catacomb

	tracerRunner *worker.Runner

	// tracerRequests is used to synchronise GetTracer
	// requests into this worker's event loop.
	tracerRequests chan traceRequest
}

// NewWorker creates a new tracer worker.
func NewWorker(cfg WorkerConfig) (*tracerWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*tracerWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "tracer",
		Clock: cfg.Clock,
		IsFatal: func(err error) bool {
			return false
		},
		RestartDelay: time.Second * 10,
		Logger:       internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &tracerWorker{
		internalStates: internalStates,
		cfg:            cfg,
		tracerRunner:   runner,
		tracerRequests: make(chan traceRequest),
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Name: "tracer",
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
	otel.SetLogger(logr.New(&loggerSink{Logger: w.cfg.Logger}))

	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	for {
		select {
		// The following ensures that all tracerRequests are serialised and
		// processed in order.
		case req := <-w.tracerRequests:
			if err := w.initTracer(ctx, req.namespace); err != nil {
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
func (w *tracerWorker) GetTracer(ctx context.Context, namespace coretrace.TracerNamespace) (coretrace.Tracer, error) {
	ns := namespace.WithTagAndKind(w.cfg.Tag, w.cfg.Kind)
	// First check if we've already got the tracer worker already running. If
	// we have, then return out quickly. The tracerRunner is the cache, so there
	// is no need to have an in-memory cache here.
	if tracer, err := w.workerFromCache(ns); err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, coretrace.ErrTracerDying
		}

		return nil, errors.Trace(err)
	} else if tracer != nil {
		return tracer, nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := traceRequest{
		namespace: ns,
		done:      make(chan error),
	}
	select {
	case w.tracerRequests <- req:
	case <-w.catacomb.Dying():
		return nil, coretrace.ErrTracerDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
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
		return nil, coretrace.ErrTracerDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.tracerRunner.Worker(ns.String(), w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return tracked.(coretrace.Tracer), nil
}

func (w *tracerWorker) workerFromCache(namespace coretrace.TaggedTracerNamespace) (coretrace.Tracer, error) {
	// If the worker already exists, return the existing worker early.
	if tracer, err := w.tracerRunner.Worker(namespace.String(), w.catacomb.Dying()); err == nil {
		return tracer.(coretrace.Tracer), nil
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

func (w *tracerWorker) initTracer(ctx context.Context, namespace coretrace.TaggedTracerNamespace) error {
	err := w.tracerRunner.StartWorker(ctx, namespace.String(), func(ctx context.Context) (worker.Worker, error) {
		return w.cfg.NewTracerWorker(
			ctx,
			namespace,
			w.cfg.Endpoint,
			w.cfg.InsecureSkipVerify,
			w.cfg.StackTracesEnabled,
			w.cfg.SampleRatio,
			w.cfg.TailSamplingThreshold,
			w.cfg.Logger,
			NewClient,
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

func (w *tracerWorker) reportInternalState(state string) {
	if w.internalStates == nil {
		return
	}
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}

// NoopWorker ensures that we get a functioning tracer even if we're not using
// it.
type noopWorker struct {
	tomb tomb.Tomb

	tracer coretrace.Tracer
}

// NewNoopWorker worker creates a worker that doesn't perform any new work on
// the context. Though it will manage the lifecycle of the worker.
func NewNoopWorker() *noopWorker {
	// Set this up, so we only ever hand out a singular tracer and span per
	// worker.
	w := &noopWorker{
		tracer: coretrace.NoopTracer{},
	}

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})

	return w
}

// GetTracer returns a tracer for the namespace.
// The noopWorker will return a stub tracer in this case.
func (w *noopWorker) GetTracer(context.Context, coretrace.TracerNamespace) (coretrace.Tracer, error) {
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

type loggerSink struct {
	Logger        logger.Logger
	name          string
	keysAndValues []any
}

// Init receives optional information about the logr library for LogSink
// implementations that need it.
func (s *loggerSink) Init(info logr.RuntimeInfo) {}

// Enabled tests whether this LogSink is enabled at the specified V-level.
// For example, commandline flags might be used to set the logging
// verbosity and disable some info logs.
func (s *loggerSink) Enabled(level int) bool {
	// From the logr docs:
	//
	//     ...levels are additive. A higher verbosity level means a log message
	//     is less important. Negative V-levels are treated as 0.
	//
	// This is the inverse of logger levels, so we need to invert the level
	// here.
	var lvl logger.Level
	switch {
	case level <= 0:
		lvl = logger.CRITICAL
	case level == 1:
		lvl = logger.ERROR
	case level == 2:
		lvl = logger.WARNING
	case level == 3:
		lvl = logger.INFO
	case level == 4:
		lvl = logger.DEBUG
	case level >= 5:
		lvl = logger.TRACE
	}
	return s.Logger.IsLevelEnabled(lvl)
}

// Info logs a non-error message with the given key/value pairs as context.
// The level argument is provided for optional logging.  This method will
// only be called when Enabled(level) is true. See Logger.Info for more
// details.
func (s *loggerSink) Info(level int, msg string, keysAndValues ...any) {
	// OpenTelemetry is very chatty, so we're going to log info statements
	// as trace statements. We can up the level if it becomes a problem.

	if !s.Logger.IsLevelEnabled(logger.TRACE) {
		return
	}

	format, args := s.formatKeysAndValues([]any{level, msg}, keysAndValues)
	s.Logger.Tracef(context.Background(), "%d: %s"+format, args...)
}

// Error logs an error, with the given message and key/value pairs as
// context.  See Logger.Error for more details.
func (s *loggerSink) Error(err error, msg string, keysAndValues ...any) {
	format, args := s.formatKeysAndValues([]any{msg, err}, keysAndValues)
	s.Logger.Errorf(context.Background(), "%s: %v"+format, args...)
}

// WithValues returns a new LogSink with additional key/value pairs.  See
// Logger.WithValues for more details.
func (s *loggerSink) WithValues(keysAndValues ...any) logr.LogSink {
	return &loggerSink{
		Logger:        s.Logger,
		name:          s.name,
		keysAndValues: append(s.keysAndValues, keysAndValues...),
	}
}

// WithName returns a new LogSink with the specified name appended.  See
// Logger.WithName for more details.
func (s *loggerSink) WithName(name string) logr.LogSink {
	return &loggerSink{
		Logger:        s.Logger,
		name:          name,
		keysAndValues: s.keysAndValues,
	}
}

func (s *loggerSink) formatKeysAndValues(init []any, keysAndValues []any) (string, []any) {
	var exprs []string

	kv := append(s.keysAndValues, keysAndValues...)
	args := init

	for _, v := range kv {
		exprs = append(exprs, "%v")
		args = append(args, v)
	}
	if len(exprs) == 0 {
		return "", args
	}

	format := ": " + strings.Join(exprs, " ")
	return format, args
}
