// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"time"

	"github.com/juju/loggo/v2"
)

// LogTailer allows for retrieval of Juju's logs.
// It first returns any matching already recorded logs and
// then waits for additional matching logs as they appear.
type LogTailer interface {
	// Logs returns the channel through which the LogTailer returns Juju logs.
	// It will be closed when the tailer stops.
	Logs() <-chan *LogRecord

	// Dying returns a channel which will be closed as the LogTailer stops.
	Dying() <-chan struct{}

	// Stop is used to request that the LogTailer stops.
	// It blocks until the LogTailer has stopped.
	Stop() error

	// Err returns the error that caused the LogTailer to stopped.
	// If it hasn't stopped or stopped without error nil will be returned.
	Err() error
}

// LogTailerParams specifies the filtering a LogTailer should
// apply to log records in order to decide which to return.
type LogTailerParams struct {
	StartID       int64
	StartTime     time.Time
	MinLevel      loggo.Level
	InitialLines  int
	NoTail        bool
	IncludeEntity []string
	ExcludeEntity []string
	IncludeModule []string
	ExcludeModule []string
	IncludeLabel  []string
	ExcludeLabel  []string
}
