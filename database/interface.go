// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import "github.com/juju/loggo"

// Logger describes methods for emitting log output.
type Logger interface {
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Debugf(string, ...interface{})

	// Logf is used to proxy Dqlite logs via this logger.
	Logf(level loggo.Level, msg string, args ...interface{})
}
