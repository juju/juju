// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux
// +build linux

package syslogger

import (
	"io"
	"log/syslog"
)

func NewSyslog(priority Priority, tag string) (io.WriteCloser, error) {
	return syslog.New(syslog.Priority(priority), tag)
}
