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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/internal/worker/syslogger"
	"github.com/juju/juju/state"
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
	StateName          string
	SyslogName         string
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
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.SyslogName == "" {
		return errors.NotValidf("empty SyslogName")
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
			config.StateName,
			config.SyslogName,
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

			var sysLogger syslogger.SysLogger
			if err := getter.Get(config.SyslogName, &sysLogger); err != nil {
				return nil, errors.Trace(err)
			}

			modelsDir := filepath.Join(currentCfg.LogDir(), "models")
			if err := os.MkdirAll(modelsDir, 0755); err != nil {
				return nil, errors.Annotate(err, "unable to create models log directory")
			}
			if err := paths.SetSyslogOwner(modelsDir); err != nil && !errors.Is(err, os.ErrPermission) {
				// If we don't have permission to chown this, it means we are running rootless.
				return nil, errors.Annotate(err, "unable to set owner for log directory")
			}

			var stTracker workerstate.StateTracker
			if err := getter.Get(config.StateName, &stTracker); err != nil {
				return nil, errors.Trace(err)
			}
			pool, _, err := stTracker.Use()
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				Logger:        config.DebugLogger,
				Clock:         clock,
				LogSinkConfig: logSinkConfig,
				LoggerForModelFunc: getLoggerForModelFunc(
					pool,
					sysLogger,
					controllerCfg.ModelLogfileMaxSizeMB(),
					controllerCfg.ModelLogfileMaxBackups(),
					config.DebugLogger,
					modelsDir,
				),
			})
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}
			return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
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
func getLoggerForModelFunc(pool *state.StatePool, sysLogger syslogger.SysLogger, maxSize, maxBackups int, debugLogger Logger, modelsDir string) corelogger.LoggerForModelFunc {
	return func(modelUUID, modelName string) (corelogger.LoggerCloser, error) {
		if !names.IsValidModel(modelUUID) {
			return nil, errors.NotValidf("model UUID %q", modelUUID)
		}
		filename := modelName + "-" + names.NewModelTag(modelUUID).ShortId() + ".log"
		logFilename := filepath.Join(modelsDir, filename)
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

		st, err := pool.Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer st.Release()

		m, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg, err := m.Config()
		if err != nil {
			return nil, errors.Trace(err)
		}
		loggingOutputs, _ := cfg.LoggingOutput()
		modelLoggers, err := getLoggers(loggingOutputs, st, sysLogger)
		if err != nil {
			return nil, errors.Annotate(err, "getting legacy loggers")
		}
		modelLoggers = append(modelLoggers, modelFileLogger)

		return corelogger.NewTeeLogger(modelLoggers...), nil
	}
}

// TODO(debug-log) - we retain the db logger for now; it will be removed.
func getLoggers(loggingOutputs []string, st state.ModelSessioner, sysLogger syslogger.SysLogger) ([]corelogger.Logger, error) {
	results := make(map[string]corelogger.Logger)
	// If the logging output is empty, then send it to state.
	if len(loggingOutputs) == 0 {
		results[config.DatabaseName] = state.NewDbLogger(st)
	}
loop:
	for _, output := range loggingOutputs {
		switch output {
		case config.SyslogName:
			results[config.SyslogName] = sysLogger
		default:
			// We only ever want one db logger.
			if _, ok := results[config.DatabaseName]; ok {
				continue loop
			}
			results[config.DatabaseName] = state.NewDbLogger(st)
		}
	}

	var loggers []corelogger.Logger
	for _, l := range results {
		loggers = append(loggers, l)
	}
	return loggers, nil
}
