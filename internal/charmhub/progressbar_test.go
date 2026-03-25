// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/tc"

	corelogger "github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type progressBarSuite struct{}

func TestProgressBarSuite(t *testing.T) {
	tc.Run(t, &progressBarSuite{})
}

func (progressBarSuite) TestWriteLogsPercentage(c *tc.C) {
	logs := captureLogs{c: c}
	logger := loggertesting.WrapCheckLogWithLevel(&logs, corelogger.TRACE)

	pb := NewLoggingProgressBar(logger)
	pb.Start("dummy", 10)

	n, err := pb.Write([]byte("abc"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(n, tc.Equals, 3)

	n, err = pb.Write([]byte("1234567"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(n, tc.Equals, 7)

	c.Check(logs.messages, tc.DeepEquals, []string{
		`TRACE: download "dummy" progress  30% complete`,
		`TRACE: download "dummy" progress 100% complete`,
	})
}

func (progressBarSuite) TestWriteLogsUnknownPercentageForZeroTotal(c *tc.C) {
	logs := captureLogs{c: c}
	logger := loggertesting.WrapCheckLogWithLevel(&logs, corelogger.TRACE)

	pb := NewLoggingProgressBar(logger)
	pb.Start("dummy", 0)

	_, err := pb.Write([]byte("abc"))
	c.Assert(err, tc.ErrorIsNil)

	c.Check(logs.messages, tc.DeepEquals, []string{
		`TRACE: download "dummy" progress 100% complete`,
	})
}

type captureLogs struct {
	c        *tc.C
	messages []string
}

func (l *captureLogs) Logf(msg string, args ...any) {
	l.messages = append(l.messages, fmt.Sprintf(msg, args...))
}

func (l *captureLogs) Context() context.Context {
	return l.c.Context()
}

func (*captureLogs) Helper() {}
