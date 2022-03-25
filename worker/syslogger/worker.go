// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger

import (
	"fmt"
	"log/syslog"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/state"
	"github.com/juju/worker/v3/catacomb"
)

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct{}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	return nil
}

type SysLogger interface {
	Log([]state.LogRecord) error
}

type syslogWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	writers map[loggo.Level]*syslog.Writer
}

var syslogLoggoLevels = map[loggo.Level]syslog.Priority{
	loggo.CRITICAL:    syslog.LOG_CRIT,
	loggo.ERROR:       syslog.LOG_ERR,
	loggo.WARNING:     syslog.LOG_WARNING,
	loggo.INFO:        syslog.LOG_INFO,
	loggo.DEBUG:       syslog.LOG_DEBUG,
	loggo.TRACE:       syslog.LOG_DEBUG, // syslog has not trace level.
	loggo.UNSPECIFIED: syslog.LOG_DEBUG,
}

func NewWorker(cfg WorkerConfig) (*syslogWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Create a writer for every log level, so we can stream line the logging
	// process.
	writers := make(map[loggo.Level]*syslog.Writer)
	for level, priority := range syslogLoggoLevels {
		writer, err := syslog.New(priority, "juju")
		if err != nil {
			return nil, errors.Trace(err)
		}
		writers[level] = writer
	}

	w := &syslogWorker{
		cfg:     cfg,
		writers: writers,
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			<-w.catacomb.Dying()
			w.close()
			return nil
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *syslogWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *syslogWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *syslogWorker) Log(logs []state.LogRecord) error {
	for _, log := range logs {
		writer, ok := w.writers[log.Level]
		if !ok {
			continue
		}
		dateTime := log.Time.In(time.UTC).Format("2006-01-02 15:04:05")
		fmt.Fprintf(writer, "%s %s %s %s\n", dateTime, log.Entity, log.Module, log.Message)
	}
	return nil
}

func (w *syslogWorker) close() {
	for _, writer := range w.writers {
		_ = writer.Close()
	}
}
