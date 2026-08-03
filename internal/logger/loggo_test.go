// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"
	"testing"

	"github.com/juju/loggo/v3"
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/testhelpers"
)

type loggoSuite struct {
	testhelpers.IsolationSuite
}

func TestLoggoSuite(t *testing.T) {
	tc.Run(t, &loggoSuite{})
}

func (s *loggoSuite) TestLog(c *tc.C) {
	cases := []struct {
		fn            func(ctx context.Context, logger logger.Logger)
		expectedLevel loggo.Level
	}{
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Criticalf(ctx, "message")
			},
			expectedLevel: loggo.CRITICAL,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Errorf(ctx, "message")
			},
			expectedLevel: loggo.ERROR,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Warningf(ctx, "message")
			},
			expectedLevel: loggo.WARNING,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Infof(ctx, "message")
			},
			expectedLevel: loggo.INFO,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Debugf(ctx, "message")
			},
			expectedLevel: loggo.DEBUG,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Tracef(ctx, "message")
			},
			expectedLevel: loggo.TRACE,
		},
	}

	for i, t := range cases {
		c.Logf("test case %d", i)

		writer := &loggo.TestWriter{}
		logContext := loggo.NewContext(loggo.TRACE)
		logContext.AddWriter("test", writer)

		logger := WrapLoggoContext(logContext)
		t.fn(c.Context(), logger.GetLogger("foo"))

		log := writer.Log()
		c.Assert(log, tc.HasLen, 1)
		c.Check(log[0].Level, tc.Equals, t.expectedLevel)
		c.Check(log[0].Module, tc.Equals, "foo")
		c.Check(log[0].Message, tc.Equals, "message")
		c.Check(log[0].Labels, tc.HasLen, 0)
	}
}

func (s *loggoSuite) TestLogWithTrace(c *tc.C) {
	cases := []struct {
		fn            func(ctx context.Context, logger logger.Logger)
		expectedLevel loggo.Level
	}{
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Criticalf(ctx, "message")
			},
			expectedLevel: loggo.CRITICAL,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Errorf(ctx, "message")
			},
			expectedLevel: loggo.ERROR,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Warningf(ctx, "message")
			},
			expectedLevel: loggo.WARNING,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Infof(ctx, "message")
			},
			expectedLevel: loggo.INFO,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Debugf(ctx, "message")
			},
			expectedLevel: loggo.DEBUG,
		},
		{
			fn: func(ctx context.Context, logger logger.Logger) {
				logger.Tracef(ctx, "message")
			},
			expectedLevel: loggo.TRACE,
		},
	}

	for i, t := range cases {
		c.Logf("test case %d", i)

		writer := &loggo.TestWriter{}
		logContext := loggo.NewContext(loggo.TRACE)
		logContext.AddWriter("test", writer)

		ctx := trace.WithTraceScope(c.Context(), "traceid", "", 0)

		logger := WrapLoggoContext(logContext)
		t.fn(ctx, logger.GetLogger("foo"))

		log := writer.Log()
		c.Assert(log, tc.HasLen, 1)
		c.Check(log[0].Level, tc.Equals, t.expectedLevel)
		c.Check(log[0].Module, tc.Equals, "foo")
		c.Check(log[0].Message, tc.Equals, "message")
		c.Check(log[0].Labels, tc.DeepEquals, loggo.Labels{
			"trace_id": "traceid",
			"span_id":  "",
		})
	}
}

func (s *loggoSuite) TestLogWithTraceAndSpan(c *tc.C) {
	writer := &loggo.TestWriter{}
	logContext := loggo.NewContext(loggo.TRACE)
	logContext.AddWriter("test", writer)

	ctx := trace.WithTraceScope(
		c.Context(),
		"0123456789abcdef0123456789abcdef",
		"0123456789abcdef",
		1,
	)

	logger := WrapLoggoContext(logContext)
	logger.GetLogger("foo").Infof(ctx, "message")

	log := writer.Log()
	c.Assert(log, tc.HasLen, 1)
	c.Check(log[0].Labels, tc.DeepEquals, loggo.Labels{
		"trace_id": "0123456789abcdef0123456789abcdef",
		"span_id":  "0123456789abcdef",
	})
}
