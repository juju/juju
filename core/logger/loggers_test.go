// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/logger/mocks"
)

type LoggersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LoggersSuite{})

func (s *LoggersSuite) TestMakeLoggersWithOneLogger(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLoggerCloser(ctrl)
	mockLogger.EXPECT().Log([]logger.LogRecord{{
		Message: "hello",
	}})

	var called bool
	loggers := logger.MakeLoggers([]string{
		logger.DatabaseName,
	}, logger.LoggersConfig{
		DBLogger: func() logger.Logger {
			called = true
			return mockLogger
		},
		SysLogger: func() logger.Logger {
			c.Fail()
			return nil
		},
	})
	c.Assert(called, gc.Equals, true)

	loggers.Log([]logger.LogRecord{{
		Message: "hello",
	}})
}

func (s *LoggersSuite) TestMakeLoggersWithMultipleLoggers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLoggerCloser(ctrl)
	mockLogger.EXPECT().Log([]logger.LogRecord{{
		Message: "hello",
	}}).Times(2)

	loggers := logger.MakeLoggers([]string{
		logger.DatabaseName,
		logger.SyslogName,
	}, logger.LoggersConfig{
		DBLogger: func() logger.Logger {
			return mockLogger
		},
		SysLogger: func() logger.Logger {
			return mockLogger
		},
	})

	loggers.Log([]logger.LogRecord{{
		Message: "hello",
	}})
}
