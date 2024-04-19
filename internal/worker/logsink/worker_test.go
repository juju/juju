// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
	corelogger "github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/testing"
)

type ModelLoggerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ModelLoggerSuite{})

func (s *ModelLoggerSuite) TestGetLogger(c *gc.C) {
	modelID := coretesting.ModelTag.Id()

	var received []request

	var testLogger stubLogger
	w, err := NewWorker(Config{
		Logger: loggo.GetLogger("test"),
		Clock:  clock.WallClock,
		LogSinkConfig: LogSinkConfig{
			LoggerFlushInterval: time.Second,
			LoggerBufferSize:    10,
		},
		LoggerForModelFunc: func(modelUUID, modelName string) (corelogger.LoggerCloser, error) {
			received = append(received, request{
				modelUUID: modelUUID,
				modelName: modelName,
			})
			return &testLogger, nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	err = w.(*LogSink).logSink.InitLogger(modelID, "foo", "fred")
	c.Assert(err, jc.ErrorIsNil)

	logger := w.(*LogSink).logSink.GetLogger(modelID)
	rec := []corelogger.LogRecord{{Message: "message1"}, {Message: "message1"}}
	err = logger.Log(rec)
	c.Assert(err, jc.ErrorIsNil)

	// We ensure there is a fallback logger.
	c.Check(received, jc.DeepEquals, []request{
		{modelUUID: database.ControllerNS, modelName: "admin-log"},
		{modelUUID: modelID, modelName: "fred-foo"},
	})

	// Closing the logger forces it to flush.
	err = logger.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testLogger.records, jc.DeepEquals, rec)
}

type request struct {
	modelUUID string
	modelName string
}
