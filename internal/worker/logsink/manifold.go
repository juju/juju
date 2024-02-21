// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// DebugLogger is used to emit debug messages.
	DebugLogger Logger

	// NewWorker creates a log sink worker.
	NewWorker func(cfg Config) (worker.Worker, error)

	// These attributes are the named workers this worker depends on.

	ClockName          string
	ServiceFactoryName string
	AgentName          string
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.DebugLogger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}

	return nil
}

// Manifold returns a dependency manifold that runs a log sink
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ServiceFactoryName,
			config.AgentName,
			config.ClockName,
		},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var controllerServiceFactory servicefactory.ControllerServiceFactory
			if err := getter.Get(config.ServiceFactoryName, &controllerServiceFactory); err != nil {
				return nil, errors.Trace(err)
			}
			controllerCfg, err := controllerServiceFactory.ControllerConfig().ControllerConfig(ctx)
			if err != nil {
				return nil, errors.Annotate(err, "cannot read controller config")
			}

			var clock clock.Clock
			if err := getter.Get(config.ClockName, &clock); err != nil {
				return nil, errors.Trace(err)
			}

			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			currentCfg := agent.CurrentConfig()
			logSinkConfig, err := getLogSinkConfig(currentCfg)
			if err != nil {
				return nil, errors.Annotate(err, "getting log sink config")
			}

			modelsDir := filepath.Join(currentCfg.LogDir(), "models")
			if err := os.MkdirAll(modelsDir, 0755); err != nil {
				return nil, errors.Annotate(err, "unable to create models log directory")
			}
			if err := paths.SetSyslogOwner(modelsDir); err != nil && !errors.Is(err, os.ErrPermission) {
				// If we don't have permission to chown this, it means we are running rootless.
				return nil, errors.Annotate(err, "unable to set owner for log directory")
			}

			w, err := config.NewWorker(Config{
				Logger:        config.DebugLogger,
				Clock:         clock,
				LogSinkConfig: logSinkConfig,
				LoggerForModelFunc: getLoggerForModelFunc(
					controllerCfg.ModelLogfileMaxSizeMB(),
					controllerCfg.ModelLogfileMaxBackups(),
					config.DebugLogger,
					currentCfg.LogDir(),
				),
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
		*outPointer = inWorker.logSink
	default:
		return errors.Errorf("out should be *corelogger.Logger; got %T", out)
	}
	return nil
}

// getLoggerForModelFunc returns a function which can be called to get a logger which can store
// logs for a specified model.
func getLoggerForModelFunc(maxSize, maxBackups int, debugLogger Logger, logDir string) corelogger.LoggerForModelFunc {
	return func(modelUUID, modelName string) (corelogger.LoggerCloser, error) {
		if !names.IsValidModel(modelUUID) {
			return nil, errors.NotValidf("model UUID %q", modelUUID)
		}
		logFilename := corelogger.ModelLogFile(logDir, modelUUID, modelName)
		if err := paths.PrimeLogFile(logFilename); err != nil && !errors.Is(err, os.ErrPermission) {
			// If we don't have permission to chown this, it means we are running rootless.
			return nil, errors.Annotate(err, "unable to prime log file")
		}
		ljLogger := &lumberjack.Logger{
			Filename:   logFilename,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			Compress:   true,
		}
		debugLogger.Debugf("created rotating log file %q with max size %d MB and max backups %d",
			ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
		modelFileLogger := &logWriter{ljLogger}
		return modelFileLogger, nil
	}
}
