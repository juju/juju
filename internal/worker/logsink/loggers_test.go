// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	corelogger "github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type LoggersSuite struct {
	testhelpers.IsolationSuite

	logWriter *MockLogSink
	modelUUID string
}

func TestLoggersSuite(t *testing.T) {
	tc.Run(t, &LoggersSuite{})
}

var _ LogSinkWriter = (*modelLogger)(nil)

func (s *LoggersSuite) TestLoggers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	logger := s.newModelLogger(c)

	workertest.CheckKill(c, logger)
}

func (s *LoggersSuite) TestLoggerLogs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.logWriter.EXPECT().Log([]corelogger.LogRecord{{Message: "foo"}}).Return(nil)

	logger := s.newModelLogger(c)
	err := logger.Log([]corelogger.LogRecord{{Message: "foo"}})
	c.Assert(err, tc.ErrorIsNil)

	workertest.CheckKill(c, logger)
}

func (s *LoggersSuite) TestLoggerGetLogger(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var logs []corelogger.LogRecord
	s.logWriter.EXPECT().Log(gomock.Any()).DoAndReturn(func(records []corelogger.LogRecord) error {
		logs = append(logs, records...)
		return nil
	})

	logger := s.newModelLogger(c)

	fooLogger := logger.GetLogger("foo")
	c.Assert(fooLogger, tc.NotNil)

	fooLogger.Infof(c.Context(), "message me")

	workertest.CheckKill(c, logger)

	c.Assert(logs, tc.HasLen, 1)
	c.Check(logs[0].Message, tc.Equals, "message me")
	c.Check(logs[0].Level, tc.Equals, corelogger.INFO)
	c.Check(logs[0].Module, tc.Equals, "foo")
	c.Check(logs[0].ModelUUID, tc.Equals, s.modelUUID)
}

func (s *LoggersSuite) TestLoggerConfigureLoggers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var logs []corelogger.LogRecord
	s.logWriter.EXPECT().Log(gomock.Any()).DoAndReturn(func(records []corelogger.LogRecord) error {
		logs = append(logs, records...)
		return nil
	})

	logger := s.newModelLogger(c)

	fooLogger := logger.GetLogger("foo")

	// The debug log, should not be logged by the logger.

	err := logger.ConfigureLoggers("<root>=INFO")
	c.Assert(err, tc.ErrorIsNil)

	fooLogger.Debugf(c.Context(), "message me")

	// Once we reset this is set to warning, so the debug log should not be
	// logged. The warning should be though.

	logger.ResetLoggerLevels()

	fooLogger.Debugf(c.Context(), "message again")
	fooLogger.Warningf(c.Context(), "message again and again")

	workertest.CheckKill(c, logger)

	c.Assert(logs, tc.HasLen, 1)
	c.Check(logs[0].Message, tc.Equals, "message again and again")
	c.Check(logs[0].Level, tc.Equals, corelogger.WARNING)
	c.Check(logs[0].ModelUUID, tc.Equals, s.modelUUID)
}

func (s *LoggersSuite) newModelLogger(c *tc.C) *modelLogger {
	s.modelUUID = uuid.MustNewUUID().String()

	w, err := NewModelLogger(s.logWriter, model.UUID(s.modelUUID), names.NewUnitTag("foo/0"))
	c.Assert(err, tc.ErrorIsNil)

	return w.(*modelLogger)
}

func (s *LoggersSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logWriter = NewMockLogSink(ctrl)

	return ctrl
}
