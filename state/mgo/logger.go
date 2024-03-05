// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mgo

import (
	"runtime"
	"strings"

	"github.com/juju/loggo/v2"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/txn"
)

const (
	mgoLoggerName    = "juju.mgo"
	mgoTxnLoggerName = "juju.mgo.txn"
)

var mgoLogger = loggo.GetLogger(mgoLoggerName)
var mgoTxnLogger = loggo.GetLogger(mgoTxnLoggerName)

// ConfigureMgoLogging sets up juju/mgo package logging according
// to the logging config value for "juju.mgo".
func ConfigureMgoLogging() {
	logLevel := mgoLogger.EffectiveLogLevel()
	// mgo logging is quite verbose.
	// mgo "debug" is similar to juju "trace".
	mgo.SetDebug(logLevel == loggo.TRACE)
	// Only output mgo logging for juju "debug" or greater.
	if logLevel == loggo.UNSPECIFIED || logLevel >= loggo.INFO {
		mgo.SetLogger(nil)
		return
	}
	mgo.SetLogger(&mgoLogWriter{
		logger: mgoLogger,
	})

	logLevel = mgoTxnLogger.EffectiveLogLevel()
	txn.SetDebug(logLevel == loggo.DEBUG)
	// Only output mgo txn logging for juju "info" or greater.
	if logLevel == loggo.UNSPECIFIED || logLevel >= loggo.WARNING {
		mgo.SetLogger(nil)
		return
	}
	txn.SetLogger(&mgoTxnLogWriter{
		logger: mgoLogger,
	})
}

type mgoLogWriter struct {
	logger loggo.Logger
}

// Output implements the mgo log_Logger interface.
func (s *mgoLogWriter) Output(calldepth int, message string) error {
	// If the output results from a debug function,
	// log at trace level.
	caller := callerFunc(calldepth - 1)
	level := loggo.DEBUG
	if strings.HasPrefix(caller, "debug") {
		level = loggo.TRACE
	}
	s.logger.LogCallf(calldepth, level, message)
	return nil
}

type mgoTxnLogWriter struct {
	logger loggo.Logger
}

// Output implements the mgo log_Logger interface.
func (s *mgoTxnLogWriter) Output(calldepth int, message string) error {
	// If the output results from a debug function,
	// log at debug level.
	caller := callerFunc(calldepth - 1)
	level := loggo.INFO
	if strings.HasPrefix(caller, "debug") {
		level = loggo.DEBUG
	}
	s.logger.LogCallf(calldepth, level, message)
	return nil
}

// callerFunc returns the name of the function
// at the specified call depth.
func callerFunc(calldepth int) string {
	var pcs [100]uintptr
	n := runtime.Callers(calldepth+2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	frame, _ := frames.Next()
	if frame.Func == nil {
		return ""
	}
	parts := strings.Split(frame.Func.Name(), ".")
	return parts[len(parts)-1]
}
