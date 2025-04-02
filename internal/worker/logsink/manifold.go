// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/common"
)

// NewModelLoggerFunc is a function that creates a new model logger.
type NewModelLoggerFunc func(logSink corelogger.LogSink, modelUUID model.UUID) (worker.Worker, error)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// LogSink is the log sink for all models.
	LogSink corelogger.LogSink

	// NewWorker creates a log sink worker.
	NewWorker func(cfg Config) (worker.Worker, error)

	// NewModelLogger creates a new model logger.
	NewModelLogger NewModelLoggerFunc

	// Clock is the clock used by the worker.
	Clock clock.Clock
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.LogSink == nil {
		return errors.NotValidf("nil LogSink")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewModelLogger == nil {
		return errors.NotValidf("nil NewModelLogger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a log sink
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				LogSink:        config.LogSink,
				Clock:          config.Clock,
				NewModelLogger: NewModelLogger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// outputFunc extracts an API connection from a *apiConnWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Unwrap()
	}
	inWorker, _ := in.(*LogSink)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *corelogger.ModelLogger:
		*outPointer = inWorker
	case *corelogger.LoggerContextGetter:
		*outPointer = inWorker
	case *corelogger.ModelLogSinkGetter:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be *logger.Logger; got %T", out)
	}
	return nil
}
