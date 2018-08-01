// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see COPYING and COPYING.LESSER file for details.

package logging

import (
	"log"

	"github.com/juju/loggo"
)

// CompatLogger is a minimal logging interface that may be provided
// when constructing a goose Client to log requests and responses,
// retaining compatibility with the old *log.Logger that was
// previously depended upon directly.
//
// TODO(axw) in goose.v2, drop this and use loggo.Logger directly.
type CompatLogger interface {
	// Printf prints a log message. Arguments are handled
	// in the/ manner of fmt.Printf.
	Printf(format string, v ...interface{})
}

// Logger is a logging interface that may be provided when constructing
// a goose Client to log requests and responses.
type Logger interface {
	Debugf(format string, v ...interface{})
	Warningf(format string, v ...interface{})
	Tracef(format string, v ...interface{})
}

// LoggoLogger is a logger that may be provided when constructing
// a goose Client to log requests and responses. Users must
// provide a CompatLogger, which will be upgraded to Logger
// if provided.
type LoggoLogger struct {
	loggo.Logger
}

// Printf is part of the CompatLogger interface.
func (l LoggoLogger) Printf(format string, v ...interface{}) {
	l.Debugf(format, v...)
}

// CompatLoggerAdapter is a type wrapping CompatLogger, implementing
// the Logger interface.
type CompatLoggerAdapter struct {
	CompatLogger
}

// Debugf is part of the Logger interface.
func (a CompatLoggerAdapter) Debugf(format string, v ...interface{}) {
	a.Printf("DEBUG: "+format, v...)
}

// Warningf is part of the Logger interface.
func (a CompatLoggerAdapter) Warningf(format string, v ...interface{}) {
	a.Printf("WARNING: "+format, v...)
}

// Tracef is part of the Logger interface.
func (a CompatLoggerAdapter) Tracef(format string, v ...interface{}) {
	a.Printf("TRACE: "+format, v...)
}

// FromCompat takes a CompatLogger, and returns a Logger. This function
// always returns a non-nil Logger; if the input is nil, then a no-op
// Logger is returned.
func FromCompat(in CompatLogger) Logger {
	if in == nil || in == (*log.Logger)(nil) {
		return CompatLoggerAdapter{nopLogger{}}
	}
	if l, ok := in.(Logger); ok {
		return l
	}
	return CompatLoggerAdapter{in}
}

type nopLogger struct{}

func (nopLogger) Printf(string, ...interface{}) {}
