// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
)

// NewModelLoggerFunc is a function that creates a new model logger.
type NewModelLoggerFunc func(ctx context.Context,
	key corelogger.LoggerKey,
	newLogWriter corelogger.LogWriterForModelFunc,
	bufferSize int,
	flushInterval time.Duration,
	clock clock.Clock) (worker.Worker, error)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// DebugLogger is used to emit debug messages.
	DebugLogger logger.Logger

	// NewWorker creates a log sink worker.
	NewWorker func(cfg Config) (worker.Worker, error)

	// NewModelLogger creates a new model logger.
	NewModelLogger NewModelLoggerFunc

	// These attributes are the named workers this worker depends on.

	ClockName          string
	DomainServicesName string
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
	if config.NewModelLogger == nil {
		return errors.NotValidf("nil NewModelLogger")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
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
			config.DomainServicesName,
			config.AgentName,
			config.ClockName,
		},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var controllerDomainServices services.ControllerDomainServices
			if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
				return nil, errors.Trace(err)
			}
			controllerCfg, err := controllerDomainServices.ControllerConfig().ControllerConfig(ctx)
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
			currentConfig := agent.CurrentConfig()
			logSinkConfig, err := getLogSinkConfig(currentConfig)
			if err != nil {
				return nil, errors.Annotate(err, "getting log sink config")
			}

			modelsDir := filepath.Join(currentConfig.LogDir(), "models")
			if err := os.MkdirAll(modelsDir, 0755); err != nil {
				return nil, errors.Annotate(err, "unable to create models log directory")
			}
			if err := paths.SetSyslogOwner(modelsDir); err != nil && !errors.Is(err, os.ErrPermission) {
				// If we don't have permission to chown this, it means we are running rootless.
				return nil, errors.Annotate(err, "unable to set owner for log directory")
			}

			w, err := config.NewWorker(Config{
				Logger:         config.DebugLogger,
				Clock:          clock,
				LogSinkConfig:  logSinkConfig,
				NewModelLogger: config.NewModelLogger,
				LogWriterForModelFunc: getLoggerForModelFunc(
					controllerCfg.ModelLogfileMaxSizeMB(),
					controllerCfg.ModelLogfileMaxBackups(),
					config.DebugLogger,
					currentConfig.LogDir(),
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
	case *logger.ModelLogger:
		*outPointer = inWorker
	case *logger.LoggerContextGetter:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be *logger.Logger; got %T", out)
	}
	return nil
}

// getLoggerForModelFunc returns a function which can be called to get a logger which can store
// logs for a specified model.
func getLoggerForModelFunc(maxSize, maxBackups int, debugLogger logger.Logger, logDir string) logger.LogWriterForModelFunc {
	return func(ctx context.Context, key corelogger.LoggerKey) (logger.LogWriterCloser, error) {
		modelUUID := key.ModelUUID

		if !names.IsValidModel(key.ModelUUID) {
			return nil, errors.NotValidf("model UUID %q", modelUUID)
		}
		logFilename := logger.ModelLogFile(logDir, key)
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
		debugLogger.Debugf(ctx, "created rotating log file %q with max size %d MB and max backups %d",
			ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
		modelFileLogger := &logWriter{WriteCloser: ljLogger}
		return modelFileLogger, nil
	}
}

// logWriter wraps a io.Writer instance and writes out
// log records to the writer.
type logWriter struct {
	io.WriteCloser
}

// Log implements logger.Log.
func (lw *logWriter) Log(records []logger.LogRecord) error {
	for _, r := range records {
		line, err := json.Marshal(&r)
		if err != nil {
			return errors.Annotatef(err, "marshalling log record")
		}
		_, err = lw.Write([]byte(fmt.Sprintf("%s\n", line)))
		if err != nil {
			return errors.Annotatef(err, "writing log record")
		}
	}
	return nil
}
