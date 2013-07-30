// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"fmt"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker"
)

var log = loggo.GetLogger("juju.worker.logger")

// Logger is responsible for watching the environment and if the
// logging-config changes, update the logging config in loggo.
type Logger struct {
	tomb       tomb.Tomb
	lastConfig string
	// Need state until the environ watcher is in the api
	st *state.State
}

// NewResumer periodically resumes pending transactions.
func NewLogger(st *state.State) worker.Worker {
	logger := &Logger{st: st}
	go func() {
		defer logger.tomb.Done()
		logger.tomb.Kill(logger.loop())
	}()
	return logger
}

func (logger *Logger) String() string {
	return fmt.Sprintf("logger")
}

func (logger *Logger) Kill() {
	logger.tomb.Kill(nil)
}

func (logger *Logger) Stop() error {
	logger.tomb.Kill(nil)
	return logger.tomb.Wait()
}

func (logger *Logger) Wait() error {
	return logger.tomb.Wait()
}

func (logger *Logger) loop() error {
	environWatcher := logger.st.WatchEnvironConfig()
	defer watcher.Stop(environWatcher, &logger.tomb)

	for {
		select {
		case <-logger.tomb.Dying():
			return tomb.ErrDying

		case config, ok := <-environWatcher.Changes():
			if !ok {
				return watcher.MustErr(environWatcher)
			}
			loggingConfig := config.LoggingConfig()
			log.Debugf("config change: %s", loggingConfig)
			if loggingConfig != logger.lastConfig {
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
	panic("unreachable")
}
