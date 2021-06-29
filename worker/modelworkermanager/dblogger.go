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

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/logdb"
)

// newModelLogger returns a buffered database logger that uses the name
// specified as the entity doing the logging.
func newModelLogger(
	name string,
	modelUUID string,
	base DBLogger,
	clock clock.Clock,
	logger Logger,
) *dbLogger {

	// Write to the database every second, or 1024 entries, whichever comes first.
	buffered := logdb.NewBufferedLogger(base, 1024, time.Second, clock)

	return &dbLogger{
		dbLogger:  base,
		buffer:    buffered,
		name:      name,
		modelUUID: modelUUID,
		logger:    logger,
	}
}

type dbLogger struct {
	dbLogger DBLogger
	buffer   *logdb.BufferedLogger

	// Use struct embedding to get the Close method.
	logdb.Logger
	// "controller-0" for machine-0 in the controller model.
	name      string
	modelUUID string
	logger    Logger
}

func (l *dbLogger) Write(entry loggo.Entry) {
	err := l.buffer.Log([]state.LogRecord{{
		Time:     entry.Timestamp,
		Entity:   l.name,
		Module:   entry.Module,
		Location: fmt.Sprintf("%s:%d", filepath.Base(entry.Filename), entry.Line),
		Level:    entry.Level,
		Message:  entry.Message,
	}})

	if err != nil {
		l.logger.Warningf("logging to DB failed for model %q, %v", l.modelUUID, err)
	}
}

func (l *dbLogger) Close() error {
	err := errors.Trace(l.buffer.Flush())
	l.dbLogger.Close()
	return err
}
