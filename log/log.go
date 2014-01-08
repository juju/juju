// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package log

import (
	"fmt"

	"launchpad.net/loggo"
)

var (
	logger = loggo.GetLogger("juju")
)

// Errorf logs a message using the ERROR priority.
func Errorf(format string, a ...interface{}) error {
	logger.Logf(loggo.ERROR, format, a...)
	return nil
}

// Warningf logs a message using the WARNING priority.
func Warningf(format string, a ...interface{}) error {
	logger.Logf(loggo.WARNING, format, a...)
	return nil
}

// Noticef logs a message using the NOTICE priority.
// Notice doesn't really convert to the loggo priorities...
func Noticef(format string, a ...interface{}) error {
	logger.Logf(loggo.INFO, format, a...)
	return nil
}

// Infof logs a message using the INFO priority.
func Infof(format string, a ...interface{}) error {
	logger.Logf(loggo.INFO, format, a...)
	return nil
}

// Debugf logs a message using the DEBUG priority.
func Debugf(format string, a ...interface{}) (err error) {
	logger.Logf(loggo.DEBUG, format, a...)
	return nil
}

// Log the error and return an error with the same text.
func LoggedErrorf(logger loggo.Logger, format string, a ...interface{}) error {
	logger.Logf(loggo.ERROR, format, a...)
	return fmt.Errorf(format, a...)
}
