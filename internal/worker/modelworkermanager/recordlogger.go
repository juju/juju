// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	corelogger "github.com/juju/juju/core/logger"
)

// newModelLogger returns a buffered database logger that uses the name
// specified as the entity doing the logging.
func newModelLogger(
	name string,
	modelUUID string,
	reclogger RecordLogger,
	clock clock.Clock,
	logger Logger,
) *recordLogger {
	// Write to the database every second, or 1024 entries, whichever comes first.
	buffered := corelogger.NewBufferedLogger(reclogger, 1024, time.Second, clock)

	return &recordLogger{
		recordLogger: reclogger,
		buffer:       buffered,
		name:         name,
		modelUUID:    modelUUID,
		logger:       logger,
	}
}

type recordLogger struct {
	recordLogger RecordLogger
	buffer       *corelogger.BufferedLogger

	// Use struct embedding to get the Close method.
	corelogger.Logger
	// "controller-0" for machine-0 in the controller model.
	name      string
	modelUUID string
	logger    Logger
}

func (l *recordLogger) Write(entry loggo.Entry) {
	err := l.buffer.Log([]corelogger.LogRecord{{
		Time:      entry.Timestamp,
		Entity:    l.name,
		Module:    entry.Module,
		Location:  fmt.Sprintf("%s:%d", filepath.Base(entry.Filename), entry.Line),
		Level:     entry.Level,
		Message:   entry.Message,
		Labels:    entry.Labels,
		ModelUUID: l.modelUUID,
	}})

	if err != nil {
		l.logger.Warningf("logging to DB failed for model %q, %v", l.modelUUID, err)
	}
}

func (l *recordLogger) Close() error {
	err := errors.Trace(l.buffer.Flush())
	l.recordLogger.Close()
	return err
}
