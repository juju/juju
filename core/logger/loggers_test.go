// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type LoggersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LoggersSuite{})

type stubLogger struct {
	LoggerCloser
	closed bool
}

func (s *stubLogger) Close() error {
	s.closed = true
	return nil
}

func (s *LoggersSuite) TestModelLoggerClose(c *gc.C) {
	loggers := map[string]LoggerCloser{
		"l1": &stubLogger{},
		"l2": &stubLogger{},
	}
	ml := NewModelLogger(
		func(modelUUID, modelName string) (LoggerCloser, error) {
			if l, ok := loggers[modelName]; ok {
				return l, nil
			}
			return nil, errors.NotFound
		},
		1, time.Millisecond, testclock.NewDilatedWallClock(time.Millisecond),
	)
	loggerToClose := ml.GetLogger("uuid1", "l1")
	loggerToRemove := ml.GetLogger("uuid2", "l2")
	err := ml.RemoveLogger("uuid2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ml.Close(), jc.ErrorIsNil)

	loggerToCheck, ok := loggerToClose.(*bufferedLoggerCloser).BufferedLogger.l.(*stubLogger)
	c.Assert(ok, jc.IsTrue)
	c.Assert(loggerToCheck.closed, jc.IsTrue)
	loggerToCheck, ok = loggerToRemove.(*bufferedLoggerCloser).BufferedLogger.l.(*stubLogger)
	c.Assert(ok, jc.IsTrue)
	c.Assert(loggerToCheck.closed, jc.IsTrue)
}
