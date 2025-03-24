// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"

	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/uuid"
)

type LoggersSuite struct {
	testing.IsolationSuite

	logWriter *MockLogSink
	modelUUID string
}

var _ = gc.Suite(&LoggersSuite{})

var _ LogSinkWriter = (*modelLogger)(nil)

func (s *LoggersSuite) TestLoggers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	logger := s.newModelLogger(c)

	workertest.CheckKill(c, logger)
}

func (s *LoggersSuite) TestLoggerLogs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.logWriter.EXPECT().Log([]corelogger.LogRecord{{Message: "foo"}}).Return(nil)

	logger := s.newModelLogger(c)
	err := logger.Log([]corelogger.LogRecord{{Message: "foo"}})
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckKill(c, logger)
}

func (s *LoggersSuite) TestLoggerGetLogger(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var logs []loggo.Entry
	s.logWriter.EXPECT().Write(gomock.Any()).DoAndReturn(func(entry loggo.Entry) {
		logs = append(logs, entry)
	})

	logger := s.newModelLogger(c)

	fooLogger := logger.GetLogger("foo")
	c.Assert(fooLogger, gc.NotNil)

	fooLogger.Infof(context.Background(), "message me")

	workertest.CheckKill(c, logger)

	c.Assert(logs, gc.HasLen, 1)
	c.Check(logs[0].Message, gc.Equals, "message me")
	c.Check(logs[0].Level, gc.Equals, loggo.INFO)
	c.Check(logs[0].Module, gc.Equals, "foo")
}

func (s *LoggersSuite) TestLoggerConfigureLoggers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var logs []loggo.Entry
	s.logWriter.EXPECT().Write(gomock.Any()).DoAndReturn(func(entry loggo.Entry) {
		logs = append(logs, entry)
	})

	logger := s.newModelLogger(c)

	fooLogger := logger.GetLogger("foo")

	// The debug log, should not be logged by the logger.

	err := logger.ConfigureLoggers("<root>=INFO")
	c.Assert(err, jc.ErrorIsNil)

	fooLogger.Debugf(context.Background(), "message me")

	// Once we reset this is set to warning, so the debug log should not be
	// logged. The warning should be though.

	logger.ResetLoggerLevels()

	fooLogger.Debugf(context.Background(), "message again")
	fooLogger.Warningf(context.Background(), "message again and again")

	workertest.CheckKill(c, logger)

	c.Assert(logs, gc.HasLen, 1)
	c.Check(logs[0].Message, gc.Equals, "message again and again")
	c.Check(logs[0].Level, gc.Equals, loggo.WARNING)
}

func (s *LoggersSuite) newModelLogger(c *gc.C) *modelLogger {
	s.modelUUID = uuid.MustNewUUID().String()

	w, err := NewModelLogger(s.logWriter)
	c.Assert(err, jc.ErrorIsNil)

	return w.(*modelLogger)
}

func (s *LoggersSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logWriter = NewMockLogSink(ctrl)

	return ctrl
}
