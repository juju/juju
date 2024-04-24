// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelogger "github.com/juju/juju/core/logger"
)

type LoggersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LoggersSuite{})

type stubLogger struct {
	corelogger.LogWriterCloser
	records []corelogger.LogRecord
	closed  bool
}

func (s *stubLogger) Log(rec []corelogger.LogRecord) error {
	s.records = append(s.records, rec...)
	return nil
}

func (s *stubLogger) Close() error {
	s.closed = true
	return nil
}

func (s *LoggersSuite) TestModelLoggerClose(c *gc.C) {
	logger1 := &stubLogger{}
	logger2 := &stubLogger{}
	loggers := map[string]corelogger.LogWriterCloser{
		"uuid1": logger1,
		"uuid2": logger2,
	}
	ml := NewModelLogger(
		func(modelUUID, modelName string) (corelogger.LogWriterCloser, error) {
			if l, ok := loggers[modelUUID]; ok {
				return l, nil
			}
			return nil, errors.NotFound
		},
		1, time.Millisecond, testclock.NewDilatedWallClock(time.Millisecond),
	)
	_, err := ml.GetLogWriter("uuid1", "l1", "fred")
	c.Assert(err, jc.ErrorIsNil)
	_, err = ml.GetLogWriter("uuid2", "l2", "fred")
	c.Assert(err, jc.ErrorIsNil)
	err = ml.RemoveLogWriter("uuid2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ml.Close(), jc.ErrorIsNil)

	c.Assert(logger1.closed, jc.IsTrue)
	c.Assert(logger2.closed, jc.IsTrue)
}
