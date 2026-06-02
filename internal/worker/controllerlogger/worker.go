// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlogger

import (
	"context"
	"slices"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// Config contains the information required for the controller logger worker
// to operate.
type Config struct {
	Context        corelogger.LoggerContext
	ModelConfigSvc ModelConfigService
	Tag            names.Tag
	Logger         corelogger.Logger
	Override       string

	UpdateAgentFunc func(string) error
}

// Validate ensures all the necessary fields have values.
func (c *Config) Validate() error {
	if c.Context == nil {
		return errors.NotValidf("missing logging context")
	}
	if c.ModelConfigSvc == nil {
		return errors.NotValidf("missing model config service")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	if c.Tag == nil {
		return errors.NotValidf("missing tag")
	}
	return nil
}

// NewWorker returns a controller-only logging worker backed by model config
// domain services.
func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	l := &loggerWorker{
		config:     config,
		lastConfig: config.Context.Config().String(),
	}
	config.Logger.Infof(ctx, "initial log config: %q", l.lastConfig)

	w, err := watcher.NewStringsWorker(watcher.StringsConfig{Handler: l})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type loggerWorker struct {
	config     Config
	lastConfig string
}

func (l *loggerWorker) SetUp(ctx context.Context) (watcher.StringsWatcher, error) {
	l.config.Logger.Infof(ctx, "controller logger worker started for %q", l.config.Tag.String())
	l.setLogging(ctx)
	return l.config.ModelConfigSvc.Watch(ctx)
}

func (l *loggerWorker) Handle(ctx context.Context, changes []string) error {
	if !slices.Contains(changes, config.LoggingConfigKey) {
		return nil
	}
	l.setLogging(ctx)
	return nil
}

func (l *loggerWorker) TearDown() error {
	l.config.Logger.Infof(context.Background(), "controller logger worker stopped")
	return nil
}

// setLogging applies the current logging configuration to the logger context.
// NOTE: This function is a near-duplicate of the setLogging method in
// internal/worker/logger/logger.go. That worker serves the same purpose but
// uses a facade (LoggerAPI) and a NotifyWatcher, whereas this one uses a
// domain service (ModelConfigService) and a StringsWatcher. Any logic
// changes here should be considered for the other implementation too.
func (l *loggerWorker) setLogging(ctx context.Context) {
	loggingConfig := ""
	logger := l.config.Logger

	if override := l.config.Override; override != "" {
		logger.Infof(ctx, "overriding logging config with override from controller config: %q", override)
		loggingConfig = override
	} else {
		cfg, err := l.config.ModelConfigSvc.ModelConfig(ctx)
		if err != nil {
			logger.Errorf(ctx, "reading controller model logging config: %v", err)
			return
		}
		loggingConfig = cfg.LoggingConfig()
	}

	if loggingConfig != l.lastConfig {
		logger.Infof(ctx, "reconfiguring logging from %q to %q", l.lastConfig, loggingConfig)
		loggerContext := l.config.Context
		loggerContext.ResetLoggerLevels()
		if err := loggerContext.ConfigureLoggers(loggingConfig); err != nil {
			logger.Warningf(ctx, "configure loggers failed: %v", err)
			_ = loggerContext.ConfigureLoggers(l.lastConfig)
			return
		}
		l.lastConfig = loggingConfig
		if callback := l.config.UpdateAgentFunc; callback != nil {
			if err := callback(loggingConfig); err != nil {
				logger.Errorf(ctx, "%v", err)
			}
		}
	}
}
