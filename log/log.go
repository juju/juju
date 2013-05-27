// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package log

import (
	"launchpad.net/loggo"
)

var (
	logger = loggo.GetLogger("juju")
)

// Errorf logs a message using the ERROR priority.
func Errorf(format string, a ...interface{}) error {
	logger.Error(format, a...)
	return nil
}

// Warningf logs a message using the WARNING priority.
func Warningf(format string, a ...interface{}) error {
	logger.Warning(format, a...)
	return nil
}

// Noticef logs a message using the NOTICE priority.
// Notice doesn't really convert to the loggo priorities...
func Noticef(format string, a ...interface{}) error {
	logger.Info(format, a...)
	return nil
}

// Infof logs a message using the INFO priority.
func Infof(format string, a ...interface{}) error {
	logger.Info(format, a...)
	return nil
}

// Debugf logs a message using the DEBUG priority.
func Debugf(format string, a ...interface{}) (err error) {
	logger.Debug(format, a...)
	return nil
}
