// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"io"
	"os"

	"github.com/juju/juju/core/auditlog"

	"github.com/juju/errors"
	"gopkg.in/natefinch/lumberjack.v2"
)

// NewFileWriter returns an io.WriteCloser that will write log messages to disk.
func NewFileWriter(logPath string) (io.WriteCloser, error) {
	if err := primeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming fails.
		logger.Warningf("Unable to prime %s (proceeding anyway): %v", logPath, err)
	}
	return &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    300, // MB
		MaxBackups: 2,
		Compress:   true,
	}, nil
}

// primeLogFile ensures the logsink log file is created with the
// correct mode and ownership.
func primeLogFile(path string) error {
	permissions := os.FileMode(0640)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, permissions)
	if err != nil {
		return errors.Trace(err)
	}
	if err := f.Close(); err != nil {
		return errors.Trace(err)
	}
	return auditlog.SetOwnerGroupLogPermissions(path, "syslog", "adm", permissions)
}
