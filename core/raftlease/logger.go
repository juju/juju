// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"fmt"
	"io"
	"log"
)

// ErrorLogger defines methods used for the logging errors.
type ErrorLogger interface {
	Errorf(string, ...interface{})
}

// TargetLogger defines a logger that writes to a persistent log (file). If
// a failure occurs, then the resulting error and original log message can be
// redirected to another logger (stderr).
type TargetLogger struct {
	leaseLogger *log.Logger
	errorLogger ErrorLogger
}

// NewTargetLogger creates a new logger which has fallback qualities.
func NewTargetLogger(leaseLogger io.Writer, errorLogger ErrorLogger) *TargetLogger {
	return &TargetLogger{
		leaseLogger: log.New(leaseLogger, "", log.LstdFlags|log.Lmicroseconds|log.LUTC),
		errorLogger: errorLogger,
	}
}

// Infof sends messages to a target lease logger, failure to write the log, will
// result in the original log message and error being sent to the fallback
// logger.
func (l *TargetLogger) Infof(message string, args ...interface{}) {
	msg := fmt.Sprintf(message, args...)
	if err := l.leaseLogger.Output(4, msg); err != nil {
		l.errorLogger.Errorf("couldn't write to lease log with messags %q: %s", msg, err.Error())
	}
}

// Errorf sends the message to the target lease logger and automatically sends
// the message to the fallback logger as well. Ensuring visibility of log
// message on errors.
func (l *TargetLogger) Errorf(message string, args ...interface{}) {
	l.Infof(message, args...)
	l.errorLogger.Errorf(message, args...)
}
