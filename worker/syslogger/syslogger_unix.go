// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package syslogger

import (
	"io"
	"log/syslog"
)

// NewSyslog creates a new instance of a syslog logger, which sends all logs to
// directly to the _local_ syslog. Returning a io.WriterCloser, so that we
// can ensure that the underlying logger can be closed when done.
func NewSyslog(priority Priority, tag string) (io.WriteCloser, error) {
	return syslog.New(syslog.Priority(priority), tag)
}
