// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// LogSinkWriter is a writer that writes log records to a log sink.
type LogSinkWriter interface {
	logger.LogWriter
	logger.LoggerContext
}

// Config defines the attributes used to create a log sink worker.
type Config struct {
	LogSink        logger.LogSink
	Clock          clock.Clock
	MachineID      string
	NewModelLogger NewModelLoggerFunc
}

// request is used to pass requests for LogSink
// instances into the worker loop.
type request struct {
	modelUUID model.UUID
	done      chan error
}

// LogSink is a worker which provides access to a log sink
// which allows log entries to be stored for specified models.
type LogSink struct {
	internalStates chan string
	catacomb       catacomb.Catacomb
	runner         *worker.Runner
	cfg            Config
	requests       chan request
}

// NewWorker returns a new worker which provides access to a log sink
// which allows log entries to be stored for specified models.
func NewWorker(cfg Config) (worker.Worker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg Config, internalState chan string) (worker.Worker, error) {
	w := &LogSink{
		cfg: cfg,
		runner: worker.NewRunner(worker.RunnerParams{
			IsFatal: func(err error) bool {
				return false
			},
			ShouldRestart: func(err error) bool {
				return !errors.Is(err, logger.ErrLoggerDying)
			},
			RestartDelay: time.Second,
			Clock:        cfg.Clock,
		}),
		requests:       make(chan request),
		internalStates: internalState,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
		},
	}); err != nil {
		return nil, errors.Annotate(err, "starting log sink worker")
	}

	return w, nil
}

// GetLogWriter returns a log writer for the specified model UUID.
// It is an error if the log writer is not running. Call InitializeLogger
// to start the log writer.
func (w *LogSink) GetLogWriter(ctx context.Context, modelUUID model.UUID) (logger.LogWriter, error) {
	sink, err := w.getLogSink(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sink, nil
}

// GetLoggerContext returns a logger context for the specified model UUID.
// It is an error if the log writer is not running. Call InitializeLogger
// to start the log writer.
func (w *LogSink) GetLoggerContext(ctx context.Context, modelUUID model.UUID) (logger.LoggerContext, error) {
	sink, err := w.getLogSink(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sink, nil
}

// Close closes all the log writers.
func (w *LogSink) Close() error {
	w.catacomb.Kill(nil)
	return w.catacomb.Wait()
}

// Kill implements Worker.Kill()
func (w *LogSink) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (w *LogSink) Wait() error {
	return w.catacomb.Wait()
}

func (w *LogSink) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case req := <-w.requests:
			err := w.initLogger(req.modelUUID)

			select {
			case req.done <- err:
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			}
		}
	}
}

func (w *LogSink) getLogSink(ctx context.Context, modelUUID model.UUID) (LogSinkWriter, error) {
	if sink, err := w.workerFromCache(modelUUID); err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, logger.ErrLoggerDying
		}
		return nil, errors.Trace(err)
	} else if sink != nil {
		return sink, nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := request{
		modelUUID: modelUUID,
		done:      make(chan error),
	}
	select {
	case w.requests <- req:
	case <-w.catacomb.Dying():
		return nil, logger.ErrLoggerDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		// If we know we've got an error, just return that error before
		// attempting to ask the LogSinkRunnerWorker.
		if err != nil {
			return nil, errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return nil, logger.ErrLoggerDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.runner.Worker(modelUUID.String(), w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if tracked == nil {
		return nil, errors.NotFoundf("logsink")
	}
	return tracked.(LogSinkWriter), nil
}

func (w *LogSink) workerFromCache(modelUUID model.UUID) (LogSinkWriter, error) {
	// If the worker already exists, return the existing worker early.
	if logsink, err := w.runner.Worker(modelUUID.String(), w.catacomb.Dying()); err == nil {
		return logsink.(LogSinkWriter), nil
	} else if errors.Is(errors.Cause(err), worker.ErrDead) {
		// Handle the case where the runner is dead due to this worker dying.
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		default:
			return nil, errors.Trace(err)
		}
	} else if !errors.Is(errors.Cause(err), errors.NotFound) {
		return nil, errors.Trace(err)
	}

	// We didn't find the worker, so return nil, we'll create it in the next
	// step.
	return nil, nil
}

func (w *LogSink) initLogger(modelUUID model.UUID) error {
	err := w.runner.StartWorker(modelUUID.String(), func() (worker.Worker, error) {
		return w.cfg.NewModelLogger(w.cfg.LogSink, modelUUID)
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

func (w *LogSink) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
