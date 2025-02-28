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

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
)

// NewModelLoggerFunc is a function that creates a new model logger.
type NewModelLoggerFunc func(ctx context.Context,
	key corelogger.LoggerKey,
	cfg ModelLoggerConfig) (worker.Worker, error)

// ModelServiceGetterFunc is a function that returns the model service.
type ModelServiceGetterFunc func(services.LogSinkServices) ModelService

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// DebugLogger is used to emit debug messages.
	DebugLogger corelogger.Logger

	// NewWorker creates a log sink worker.
	NewWorker func(cfg Config) (worker.Worker, error)

	// NewModelLogger creates a new model logger.
	NewModelLogger NewModelLoggerFunc

	// ModelServiceGetter returns the model service.
	ModelServiceGetter ModelServiceGetterFunc

	// These attributes are the named workers this worker depends on.

	ClockName       string
	LogSinkServices string
	AgentName       string
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
	if config.ModelServiceGetter == nil {
		return errors.NotValidf("nil ModelServiceGetter")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.LogSinkServices == "" {
		return errors.NotValidf("empty LogSinkServices")
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
			config.LogSinkServices,
			config.AgentName,
			config.ClockName,
		},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var logSinkServices services.LogSinkServices
			if err := getter.Get(config.LogSinkServices, &logSinkServices); err != nil {
				return nil, errors.Trace(err)
			}

			controllerCfg, err := logSinkServices.ControllerConfig().ControllerConfig(ctx)
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

			machineID := agent.CurrentConfig().Tag().Id()

			w, err := config.NewWorker(Config{
				Logger:         config.DebugLogger,
				Clock:          clock,
				LogSinkConfig:  logSinkConfig,
				MachineID:      machineID,
				NewModelLogger: config.NewModelLogger,
				LogWriterForModelFunc: getLoggerForModelFunc(
					controllerCfg.ModelLogfileMaxSizeMB(),
					controllerCfg.ModelLogfileMaxBackups(),
					config.DebugLogger,
					currentConfig.LogDir(),
				),
				ModelService: config.ModelServiceGetter(logSinkServices),
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

// getLoggerForModelFunc returns a function which can be called to get a logger which can store
// logs for a specified model.
func getLoggerForModelFunc(maxSize, maxBackups int, debugLogger corelogger.Logger, logDir string) corelogger.LogWriterForModelFunc {
	return func(ctx context.Context, key corelogger.LoggerKey) (corelogger.LogWriterCloser, error) {
		modelUUID := key.ModelUUID

		if !names.IsValidModel(key.ModelUUID) {
			return nil, errors.NotValidf("model UUID %q", modelUUID)
		}
		logFilename := corelogger.ModelLogFile(logDir, key)
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
func (lw *logWriter) Log(records []corelogger.LogRecord) error {
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

// NewModelService returns a new model service.
func NewModelService(services services.LogSinkServices) ModelService {
	return modelService{
		services: services,
	}
}

type modelService struct {
	services services.LogSinkServices
}

// Model returns the model information.
func (s modelService) Model(ctx context.Context, modelUUID model.UUID) (model.ModelInfo, error) {
	return s.services.Model().Model(ctx, modelUUID)
}
