// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api/logger"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/worker"
)

var log = loggo.GetLogger("juju.worker.logger")

// Cleaner is responsible for cleaning up the state.
type Logger struct {
	api        *logger.State
	tag        string
	lastConfig string
}

var _ worker.NotifyWatchHandler = (*Logger)(nil)

// NewLogger returns a worker.Worker that runs state.Cleanup()
// if the CleanupWatcher signals documents marked for deletion.
func NewLogger(api *logger.State, agentConfig agent.Config) worker.Worker {
	logger := &Logger{
		api:        api,
		tag:        agentConfig.Tag(),
		lastConfig: loggo.LoggerInfo(),
	}
	log.Debugf("initial log config: %q", logger.lastConfig)
	return worker.NewNotifyWorker(logger)
}

func (logger *Logger) setLogging() {
	loggingConfig, err := logger.api.LoggingConfig(logger.tag)
	if err != nil {
		log.Errorf("%v", err)
	} else {
		if loggingConfig != logger.lastConfig {
			log.Debugf("reconfigurint logging from %q to %q", logger.lastConfig, loggingConfig)
			loggo.ResetLoggers()
			if err := loggo.ConfigureLoggers(loggingConfig); err != nil {
				// This shouldn't occur as the loggingConfig should be
				// validated by the original Config before it gets here.
				log.Warningf("configure loggers failed: %v", err)
				// Try to reset to what we had before
				loggo.ConfigureLoggers(logger.lastConfig)
			}
			logger.lastConfig = loggingConfig
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

func (logger *Logger) Handle() error {
	logger.setLogging()
	return nil
}

func (logger *Logger) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	return nil
}
