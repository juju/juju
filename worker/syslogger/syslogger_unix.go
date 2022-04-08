// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux
// +build linux

package syslogger

import (
	"io"
	"io/ioutil"
	"log/syslog"
)

// NewSyslog creates a new instance of a syslog logger, which sends all logs to
// directly to the _local_ syslog. Returning a io.WriterCloser, so that we
// can ensure that the underlying logger can be closed when done.
func NewSyslog(priority Priority, tag string) (io.WriteCloser, error) {
	return syslog.New(syslog.Priority(priority), tag)
}

// NewDiscard creates a new WriteCloser that discards all writes and the close
// is a noop.
func NewDiscard(priority Priority, tag string) (io.WriteCloser, error) {
	return nopCloser{
		Writer: ioutil.Discard,
	}, nil
}

// nopCloser is a closer that discards the close request. We can't use the
// io.NopCloser as that expects a io.Reader.
type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error {
	return nil
}
