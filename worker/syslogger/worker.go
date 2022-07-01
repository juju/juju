// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger

import (
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	corelogger "github.com/juju/juju/v2/core/logger"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
)

// NewLogger is a factory function to create a new syslog logger.
type NewLogger func(Priority, string) (io.WriteCloser, error)

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	NewLogger NewLogger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.NewLogger == nil {
		return errors.NotValidf("nil NewLogger")
	}
	return nil
}

type Priority int

const (
	// Severity.

	// From /usr/include/sys/syslog.h.
	// These are the same on Linux, BSD, and OS X.
	LOG_EMERG Priority = iota
	LOG_ALERT
	LOG_CRIT
	LOG_ERR
	LOG_WARNING
	LOG_NOTICE
	LOG_INFO
	LOG_DEBUG
)

// SysLogger defines an interface for logging log records.
type SysLogger interface {
	Log([]corelogger.LogRecord) error
}

type syslogWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	writers map[loggo.Level]io.WriteCloser
}

var syslogLoggoLevels = map[loggo.Level]Priority{
	loggo.CRITICAL:    LOG_CRIT,
	loggo.ERROR:       LOG_ERR,
	loggo.WARNING:     LOG_WARNING,
	loggo.INFO:        LOG_INFO,
	loggo.DEBUG:       LOG_DEBUG,
	loggo.TRACE:       LOG_DEBUG, // syslog has not trace level.
	loggo.UNSPECIFIED: LOG_DEBUG,
}

func NewWorker(cfg WorkerConfig) (worker.Worker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Create a writer for every log level, so we can stream line the logging
	// process.
	writers := make(map[loggo.Level]io.WriteCloser)
	for level, priority := range syslogLoggoLevels {
		writer, err := cfg.NewLogger(priority, "juju.daemon")
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

func (w *syslogWorker) Log(logs []corelogger.LogRecord) error {
	// Prevent logging out if the worker has already been killed.
	select {
	case <-w.catacomb.Dead():
		return w.catacomb.Err()
	default:
	}

	for _, log := range logs {
		writer, ok := w.writers[log.Level]
		if !ok {
			continue
		}
		module := log.Module
		if names.IsValidModel(log.ModelUUID) {
			module = fmt.Sprintf("%s.%s", log.Module, names.NewModelTag(log.ModelUUID).ShortId())
		}
		dateTime := log.Time.In(time.UTC).Format("2006-01-02 15:04:05")
		_, _ = fmt.Fprintf(writer, "%s %s %s %s\n", dateTime, log.Entity, module, log.Message)
	}
	return nil
}

func (w *syslogWorker) close() {
	for _, writer := range w.writers {
		_ = writer.Close()
	}
}
