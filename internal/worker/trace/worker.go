// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"go.opentelemetry.io/otel"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"

	defaultOpenTelemetrySampleRatio           = 0.1
	defaultOpenTelemetryTailSamplingThreshold = time.Millisecond
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

	Enabled               bool
	Endpoint              string
	InsecureSkipVerify    bool
	StackTracesEnabled    bool
	SampleRatio           float64
	TailSamplingThreshold time.Duration

	RuntimeConfigProvider RuntimeConfigProvider
}

// RuntimeConfigProvider defines a source of runtime tracing config updates.
// The worker uses this to get initial config and watch for subsequent updates.
type RuntimeConfigProvider interface {
	CurrentRuntimeConfig(context.Context) (RuntimeConfig, error)
	WatchRuntimeConfig(context.Context) (watcher.NotifyWatcher, error)
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
	if c.RuntimeConfigProvider == nil {
		return errors.NotValidf("nil RuntimeConfigProvider")
	}
	// If tracing is enabled, then we require an endpoint.
	if c.Enabled && c.Endpoint == "" {
		return errors.NotValidf("empty Endpoint")
	}
	return nil
}

// RuntimeConfig defines mutable tracing runtime configuration.
type RuntimeConfig struct {
	Enabled               bool
	Endpoint              string
	InsecureSkipVerify    bool
	StackTracesEnabled    bool
	SampleRatio           float64
	TailSamplingThreshold time.Duration
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
	configMu       sync.RWMutex
	runtimeConfig  RuntimeConfig
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
		runtimeConfig: RuntimeConfig{
			Enabled:               cfg.Enabled,
			Endpoint:              cfg.Endpoint,
			InsecureSkipVerify:    cfg.InsecureSkipVerify,
			StackTracesEnabled:    cfg.StackTracesEnabled,
			SampleRatio:           cfg.SampleRatio,
			TailSamplingThreshold: cfg.TailSamplingThreshold,
		},
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

	ctx := w.catacomb.Context(context.Background())

	var runtimeConfigChanges <-chan struct{}
	if err := w.reloadRuntimeConfig(ctx); err != nil {
		return errors.Trace(err)
	}

	runtimeConfigWatcher, err := w.cfg.RuntimeConfigProvider.WatchRuntimeConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if runtimeConfigWatcher != nil {
		if err := w.catacomb.Add(runtimeConfigWatcher); err != nil {
			return errors.Trace(err)
		}
		runtimeConfigChanges = runtimeConfigWatcher.Changes()
	}

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

		case _, ok := <-runtimeConfigChanges:
			if !ok {
				return errors.New("runtime config watcher channel closed")
			}
			if err := w.reloadRuntimeConfig(ctx); err != nil {
				return errors.Trace(err)
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
	if runtimeCfg := w.getRuntimeConfig(); !runtimeCfg.Enabled || runtimeCfg.Endpoint == "" {
		return coretrace.NoopTracer{}, nil
	}

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
		done:      make(chan error, 1),
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
		if errors.Is(errors.Cause(err), errors.NotFound) {
			if runtimeCfg := w.getRuntimeConfig(); !runtimeCfg.Enabled || runtimeCfg.Endpoint == "" {
				return coretrace.NoopTracer{}, nil
			}
		}
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
	runtimeCfg := w.getRuntimeConfig()
	if !runtimeCfg.Enabled || runtimeCfg.Endpoint == "" {
		return nil
	}

	err := w.tracerRunner.StartWorker(ctx, namespace.String(), func(ctx context.Context) (worker.Worker, error) {
		return w.cfg.NewTracerWorker(
			ctx,
			namespace,
			runtimeCfg.Endpoint,
			runtimeCfg.InsecureSkipVerify,
			runtimeCfg.StackTracesEnabled,
			runtimeCfg.SampleRatio,
			runtimeCfg.TailSamplingThreshold,
			w.cfg.Logger,
			NewClient,
		)
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

func (w *tracerWorker) applyConfig(config RuntimeConfig) error {
	config = normalizeRuntimeConfig(config)
	current := w.getRuntimeConfig()
	w.setRuntimeConfig(config)

	if !runtimeConfigChanged(current, config) {
		return nil
	}

	return w.stopTrackedTracers()
}

func normalizeRuntimeConfig(config RuntimeConfig) RuntimeConfig {
	// Endpoint is required when enabled. If an endpoint is not present, force
	// the worker into disabled mode.
	if config.Endpoint == "" {
		config.Enabled = false
	}
	return config
}

func runtimeConfigChanged(current, next RuntimeConfig) bool {
	return current != next
}

func (w *tracerWorker) stopTrackedTracers() error {
	for _, workerName := range w.tracerRunner.WorkerNames() {
		err := w.tracerRunner.StopAndRemoveWorker(workerName, w.catacomb.Dying())
		if errors.Is(errors.Cause(err), errors.NotFound) {
			continue
		}
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (w *tracerWorker) reloadRuntimeConfig(ctx context.Context) error {
	runtimeConfig, err := w.cfg.RuntimeConfigProvider.CurrentRuntimeConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return w.applyConfig(runtimeConfig)
}

func (w *tracerWorker) getRuntimeConfig() RuntimeConfig {
	w.configMu.RLock()
	defer w.configMu.RUnlock()
	return w.runtimeConfig
}

func (w *tracerWorker) setRuntimeConfig(config RuntimeConfig) {
	w.configMu.Lock()
	defer w.configMu.Unlock()
	w.runtimeConfig = config
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
