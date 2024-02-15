// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	corelogger "github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/testing"
)

type ModelLoggerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ModelLoggerSuite{})

func (s *ModelLoggerSuite) TestGetLogger(c *gc.C) {
	testLogger := stubLogger{}
	ml, err := NewWorker(Config{
		Logger: loggo.GetLogger("test"),
		Clock:  testclock.NewDilatedWallClock(time.Millisecond),
		LogSinkConfig: LogSinkConfig{
			LoggerFlushInterval: time.Second,
			LoggerBufferSize:    10,
		},
		LoggerForModelFunc: func(modelUUID, modelName string) (corelogger.LoggerCloser, error) {
			c.Assert(modelUUID, gc.Equals, coretesting.ModelTag.Id())
			c.Assert(modelName, gc.Equals, "foo")
			return &testLogger, nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, ml)

	logger, err := ml.(*LogSink).logSink.GetLogger(coretesting.ModelTag.Id(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	rec := []corelogger.LogRecord{{Message: "message1"}, {Message: "message1"}}
	err = logger.Log(rec)
	c.Assert(err, jc.ErrorIsNil)
	// Closing the logger forces it to flush.
	err = logger.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testLogger.records, jc.DeepEquals, rec)
}
