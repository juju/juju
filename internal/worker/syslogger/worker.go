// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger

import (
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	corelogger "github.com/juju/juju/core/logger"
)

// NewLogger is a factory function to create a new syslog logger.
type NewLogger func(Priority, string) (io.WriteCloser, error)

// WorkerConfig encapsulates the configuration options for the
// syslogger worker.
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
	tomb tomb.Tomb
	cfg  WorkerConfig

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

	w.tomb.Go(func() error {
		defer w.close()

		<-w.tomb.Dying()
		return tomb.ErrDying
	})

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *syslogWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *syslogWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *syslogWorker) Log(logs []corelogger.LogRecord) error {
	// Prevent logging out if the worker has already been killed.
	select {
	case <-w.tomb.Dying():
		return w.tomb.Err()
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
