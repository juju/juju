// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger

import (
	"io"
)

// NewDiscard creates a new WriteCloser that discards all writes and the close
// is a noop.
func NewDiscard(priority Priority, tag string) (io.WriteCloser, error) {
	return nopCloser{
		Writer: io.Discard,
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
