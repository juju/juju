// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/logger"
	"github.com/juju/juju/watcher"
)

var log = loggo.GetLogger("juju.worker.logger")

// Logger is responsible for updating the loggo configuration when the
// environment watcher tells the agent that the value has changed.
type Logger struct {
	api            *logger.State
	tag            names.Tag
	updateCallback func(string) error
	lastConfig     string
	configOverride string
}

// NewLogger returns a worker.Worker that uses the notify watcher returned
// from the setup.
func NewLogger(api *logger.State, tag names.Tag, loggingOverride string, updateCallback func(string) error) (worker.Worker, error) {
	logger := &Logger{
		api:            api,
		tag:            tag,
		updateCallback: updateCallback,
		lastConfig:     loggo.LoggerInfo(),
		configOverride: loggingOverride,
	}
	log.Debugf("initial log config: %q", logger.lastConfig)

	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (logger *Logger) setLogging() {
	loggingConfig := ""

	if logger.configOverride != "" {
		log.Debugf("overriding logging config with override from agent.conf %q", logger.configOverride)
		loggingConfig = logger.configOverride
	} else {
		modelLoggingConfig, err := logger.api.LoggingConfig(logger.tag)
		if err != nil {
			log.Errorf("%v", err)
			return
		}
		loggingConfig = modelLoggingConfig
	}

	if loggingConfig != logger.lastConfig {
		log.Debugf("reconfiguring logging from %q to %q", logger.lastConfig, loggingConfig)
		loggo.DefaultContext().ResetLoggerLevels()
		if err := loggo.ConfigureLoggers(loggingConfig); err != nil {
			// This shouldn't occur as the loggingConfig should be
			// validated by the original Config before it gets here.
			log.Warningf("configure loggers failed: %v", err)
			// Try to reset to what we had before
			loggo.ConfigureLoggers(logger.lastConfig)
			return
		}
		logger.lastConfig = loggingConfig
		// Save the logging config in the agent.conf file.
		if logger.updateCallback != nil {
			err := logger.updateCallback(loggingConfig)
			if err != nil {
				log.Errorf("%v", err)
			}
		}
	}
}

func (logger *Logger) SetUp() (watcher.NotifyWatcher, error) {
	log.Debugf("logger setup")
	// We need to set this up initially as the NotifyWorker sucks up the first
	// event.
	logger.setLogging()
	return logger.api.WatchLoggingConfig(logger.tag)
}

func (logger *Logger) Handle(_ <-chan struct{}) error {
	logger.setLogging()
	return nil
}

func (logger *Logger) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	return nil
}
