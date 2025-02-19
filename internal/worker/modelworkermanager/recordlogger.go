// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/juju/loggo/v2"

	"github.com/juju/juju/core/logger"
)

// newModelLogger returns a buffered database logger that uses the name
// specified as the entity doing the logging.
func newModelLogger(
	name string,
	modelUUID string,
	reclogger RecordLogger,
	logger logger.Logger,
) *recordLogger {
	return &recordLogger{
		recordLogger: reclogger,
		name:         name,
		modelUUID:    modelUUID,
		logger:       logger,
	}
}

type recordLogger struct {
	recordLogger RecordLogger
	io.Closer

	// "controller-0" for machine-0 in the controller model.
	name      string
	modelUUID string
	logger    logger.Logger
}

func (l *recordLogger) Write(entry loggo.Entry) {
	err := l.recordLogger.Log([]logger.LogRecord{{
		Time:      entry.Timestamp,
		Entity:    l.name,
		Module:    entry.Module,
		Location:  fmt.Sprintf("%s:%d", filepath.Base(entry.Filename), entry.Line),
		Level:     logger.Level(entry.Level),
		Message:   entry.Message,
		Labels:    entry.Labels,
		ModelUUID: l.modelUUID,
	}})

	if err != nil {
		l.logger.Warningf(context.Background(), "writing model logs failed for model %q, %v", l.modelUUID, err)
	}
}

func (l *recordLogger) Close() error {
	return l.recordLogger.Close()
}
