// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import "github.com/juju/loggo/v2"

// LoggerFactory is the interface that is used to create new loggers.
type LoggerFactory interface {
	Child(string) Logger
	ChildWithTags(string, ...string) Logger
}

// Logger is the interface to use for logging requests and errors.
type Logger interface {
	IsTraceEnabled() bool

	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// LoggoLoggerFactory is a LoggerFactory that creates loggers using
// the loggo package.
func LoggoLoggerFactory(logger loggo.Logger) LoggerFactory {
	return loggoLoggerFactory{
		logger: logger,
	}
}

type loggoLoggerFactory struct {
	logger loggo.Logger
}

func (f loggoLoggerFactory) Child(name string) Logger {
	return f.logger.Child(name)
}

func (f loggoLoggerFactory) ChildWithTags(name string, labels ...string) Logger {
	return f.logger.ChildWithTags(name, labels...)
}
