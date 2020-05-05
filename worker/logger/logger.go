// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/core/watcher"
)

// LoggerAPI represents the API calls the logger makes.
type LoggerAPI interface {
	LoggingConfig(agentTag names.Tag) (string, error)
	WatchLoggingConfig(agentTag names.Tag) (watcher.NotifyWatcher, error)
}

// WorkerConfig contains the information required for the Logger worker
// to operate.
type WorkerConfig struct {
	Context  *loggo.Context
	API      LoggerAPI
	Tag      names.Tag
	Logger   Logger
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
	config.Logger.Debugf("initial log config: %q", logger.lastConfig)

	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (l *loggerWorker) setLogging() {
	loggingConfig := ""
	logger := l.config.Logger

	if override := l.config.Override; override != "" {
		logger.Debugf("overriding logging config with override from agent.conf %q", override)
		loggingConfig = override
	} else {
		modelLoggingConfig, err := l.config.API.LoggingConfig(l.config.Tag)
		if err != nil {
			logger.Errorf("%v", err)
			return
		}
		loggingConfig = modelLoggingConfig
	}

	if loggingConfig != l.lastConfig {
		logger.Debugf("reconfiguring logging from %q to %q", l.lastConfig, loggingConfig)
		context := l.config.Context
		context.ResetLoggerLevels()
		if err := context.ConfigureLoggers(loggingConfig); err != nil {
			// This shouldn't occur as the loggingConfig should be
			// validated by the original Config before it gets here.
			logger.Warningf("configure loggers failed: %v", err)
			// Try to reset to what we had before
			context.ConfigureLoggers(l.lastConfig)
			return
		}
		l.lastConfig = loggingConfig
		// Save the logging config in the agent.conf file.
		if callback := l.config.Callback; callback != nil {
			err := callback(loggingConfig)
			if err != nil {
				logger.Errorf("%v", err)
			}
		}
	}
}

// SetUp is called by the NotifyWorker when the worker starts, and it is
// required to return a notify watcher that is used as the event source
// for the Handle method.
func (l *loggerWorker) SetUp() (watcher.NotifyWatcher, error) {
	l.config.Logger.Infof("logger worker started")
	// We need to set this up initially as the NotifyWorker sucks up the first
	// event.
	l.setLogging()
	return l.config.API.WatchLoggingConfig(l.config.Tag)
}

// Handle is called by the NotifyWorker whenever the notify event is fired.
func (l *loggerWorker) Handle(_ <-chan struct{}) error {
	l.setLogging()
	return nil
}

// TearDown is called by the NotifyWorker when the worker is being stopped.
func (l *loggerWorker) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	l.config.Logger.Infof("logger worker stopped")
	return nil
}
