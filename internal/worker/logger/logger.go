// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// LoggerAPI represents the API calls the logger makes.
type LoggerAPI interface {
	LoggingConfig(ctx context.Context, agentTag names.Tag) (string, error)
	WatchLoggingConfig(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error)
}

// WorkerConfig contains the information required for the Logger worker
// to operate.
type WorkerConfig struct {
	Context  logger.LoggerContext
	API      LoggerAPI
	Tag      names.Tag
	Logger   logger.Logger
	Override string

	Callback func(string) error
}

// Validate ensures all the necessary fields have values.
func (c *WorkerConfig) Validate() error {
	if c.Context == nil {
		return errors.NotValidf("missing logging context")
	}
	if c.API == nil {
		return errors.NotValidf("missing api")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

// loggerWorker is responsible for updating the loggo configuration when the
// environment watcher tells the agent that the value has changed.
type loggerWorker struct {
	config     WorkerConfig
	lastConfig string
}

// NewLogger returns a worker.Worker that uses the notify watcher returned
// from the setup.
func NewLogger(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	logger := &loggerWorker{
		config:     config,
		lastConfig: config.Context.Config().String(),
	}
	config.Logger.Debugf(context.Background(), "initial log config: %q", logger.lastConfig)

	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (l *loggerWorker) setLogging(ctx context.Context) {
	loggingConfig := ""
	logger := l.config.Logger

	if override := l.config.Override; override != "" {
		logger.Debugf(ctx, "overriding logging config with override from agent.conf %q", override)
		loggingConfig = override
	} else {
		modelLoggingConfig, err := l.config.API.LoggingConfig(ctx, l.config.Tag)
		if err != nil {
			logger.Errorf(ctx, "%v", err)
			return
		}
		loggingConfig = modelLoggingConfig
	}

	if loggingConfig != l.lastConfig {
		logger.Debugf(ctx, "reconfiguring logging from %q to %q", l.lastConfig, loggingConfig)
		loggerContext := l.config.Context
		loggerContext.ResetLoggerLevels()
		if err := loggerContext.ConfigureLoggers(loggingConfig); err != nil {
			// This shouldn't occur as the loggingConfig should be
			// validated by the original Config before it gets here.
			logger.Warningf(ctx, "configure loggers failed: %v", err)
			// Try to reset to what we had before
			_ = loggerContext.ConfigureLoggers(l.lastConfig)
			return
		}
		l.lastConfig = loggingConfig
		// Save the logging config in the agent.conf file.
		if callback := l.config.Callback; callback != nil {
			err := callback(loggingConfig)
			if err != nil {
				logger.Errorf(ctx, "%v", err)
			}
		}
	}
}

// SetUp is called by the NotifyWorker when the worker starts, and it is
// required to return a notify watcher that is used as the event source
// for the Handle method.
func (l *loggerWorker) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	l.config.Logger.Infof(ctx, "logger worker started")
	// We need to set this up initially as the NotifyWorker sucks up the first
	// event.
	l.setLogging(ctx)
	return l.config.API.WatchLoggingConfig(ctx, l.config.Tag)
}

// Handle is called by the NotifyWorker whenever the notify event is fired.
func (l *loggerWorker) Handle(ctx context.Context) error {
	l.setLogging(ctx)
	return nil
}

// TearDown is called by the NotifyWorker when the worker is being stopped.
func (l *loggerWorker) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	l.config.Logger.Infof(context.Background(), "logger worker stopped")
	return nil
}
