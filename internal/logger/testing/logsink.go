// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/logger"
)

type CheckLogSink struct {
	c CheckLogger
}

// WrapCheckLogSink returns a CheckLogSink that logs to the given *testing.T.
func WrapCheckLogSink(c CheckLogger) *CheckLogSink {
	return &CheckLogSink{c: c}
}

// Log writes the given log records to the logger's storage.
func (s *CheckLogSink) Log(records []logger.LogRecord) error {
	for _, record := range records {
		s.c.Logf("%s %s: %s", record.Time.Format(time.RFC3339), record.Level.String(), record.Message)
	}
	return nil
}

// Write writes a message to the Writer with the given level and module
// name. The filename and line hold the file name and line number of the
// code that is generating the log message; the time stamp holds the time
// the log message was generated, and message holds the log message
// itself.
func (s *CheckLogSink) Write(entry loggo.Entry) {
	s.c.Logf("%s %s: %s", entry.Timestamp.Format(time.RFC3339), entry.Level.String(), entry.Message)
}
