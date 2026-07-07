// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/common"
)

// NewModelLoggerFunc is a function that creates a new model logger.
type NewModelLoggerFunc func(corelogger.LogSink, model.UUID, names.Tag) (worker.Worker, error)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// AgentTag is the tag of the agent, this is used for setting the entity
	// tag on the logs.
	AgentTag names.Tag

	// LogRouterName is the name of the log-router manifold dependency.
	// The log router provides the active LogSink (which may forward to
	// Loki or the controller logsink) and signals refreshes when the
	// backend changes.
	LogRouterName string

	// NewWorker creates a log sink worker.
	NewWorker func(cfg Config) (worker.Worker, error)

	// NewModelLogger creates a new model logger.
	NewModelLogger NewModelLoggerFunc
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.LogRouterName == "" {
		return errors.NotValidf("empty LogRouterName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewModelLogger == nil {
		return errors.NotValidf("nil NewModelLogger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a log sink
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.LogRouterName},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var logSink corelogger.LogSink
			if err := getter.Get(config.LogRouterName, &logSink); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				AgentTag:       config.AgentTag,
				LogRouter:      StaticLogRouter(logSink),
				Clock:          clock.WallClock,
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
func outputFunc(in worker.Worker, out any) error {
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
