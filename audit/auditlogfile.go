// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger = loggo.GetLogger("juju.audit")

// NewLogFileSink returns an audit entry sink which writes
// to an audit.log file in the specified directory.
func NewLogFileSink(logDir string) AuditEntrySinkFn {
	logPath := filepath.Join(logDir, "audit.log")
	if err := primeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming
		// fails.
		logger.Errorf("Unable to prime %s (proceeding anyway): %v", logPath, err)
	}

	handler := &auditLogFileSink{
		fileLogger: &lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    300, // MB
			MaxBackups: 10,
		},
	}
	return handler.handle
}

// primeLogFile ensures the logsink log file is created with the
// correct mode and ownership.
func primeLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Trace(err)
	}
	if err := f.Close(); err != nil {
		return errors.Trace(err)
	}
	err = utils.ChownPath(path, "syslog")
	return errors.Trace(err)
}

type auditLogFileSink struct {
	fileLogger io.WriteCloser
}

func (a *auditLogFileSink) handle(entry AuditEntry) error {
	_, err := a.fileLogger.Write([]byte(strings.Join([]string{
		entry.Timestamp.In(time.UTC).Format("2006-01-02 15:04:05"),
		entry.ModelUUID,
		entry.RemoteAddress,
		entry.OriginName,
		entry.OriginType,
		entry.Operation,
		fmt.Sprintf("%v", entry.Data),
	}, ",") + "\n"))
	return err
}
