// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package syslogger

import (
	"io"
)

func NewSyslog(priority Priority, tag string) (io.WriteCloser, error) {
	return closer{io.Discard}, nil
}

type closer struct {
	io.Writer
}

func (closer) Close() error { return nil }
