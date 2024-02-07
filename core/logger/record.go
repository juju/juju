// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/version/v2"
)

// LogRecord defines a single Juju log message as returned by
// LogTailer.
type LogRecord struct {
	// universal fields
	ID   int64
	Time time.Time

	// origin fields
	ModelUUID string
	Entity    string
	Version   version.Number

	// logging-specific fields
	Level    loggo.Level
	Module   string
	Location string
	Message  string
	Labels   []string
}
