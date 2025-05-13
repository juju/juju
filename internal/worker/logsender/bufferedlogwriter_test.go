// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package logsender_test

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	internallogger "github.com/juju/juju/internal/logger"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/logsender/logsendertest"
)

const maxLen = 6

type bufferedLogWriterSuite struct {
	coretesting.BaseSuite
	writer      *logsender.BufferedLogWriter
	shouldClose bool
}

var _ = tc.Suite(&bufferedLogWriterSuite{})

func (s *bufferedLogWriterSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.writer = logsender.NewBufferedLogWriter(maxLen)
	s.shouldClose = true
}

func (s *bufferedLogWriterSuite) TearDownTest(c *tc.C) {
	if s.shouldClose {
		s.writer.Close()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *bufferedLogWriterSuite) TestOne(c *tc.C) {
	s.writeAndReceive(c)
}

func (s *bufferedLogWriterSuite) TestMultiple(c *tc.C) {
	for i := 0; i < 10; i++ {
		s.writeAndReceive(c)
	}
}

func (s *bufferedLogWriterSuite) TestBuffering(c *tc.C) {
	// Write several log message before attempting to read them out.
	const numMessages = 5
	now := time.Now()
	for i := 0; i < numMessages; i++ {
		s.writer.Write(
			loggo.Entry{
				Level:     loggo.Level(i),
				Module:    fmt.Sprintf("module%d", i),
				Filename:  fmt.Sprintf("filename%d", i),
				Line:      i,
				Timestamp: now.Add(time.Duration(i)),
				Message:   fmt.Sprintf("message%d", i),
			})
	}

	for i := 0; i < numMessages; i++ {
		c.Assert(*s.receiveOne(c), tc.DeepEquals, logsender.LogRecord{
			Time:     now.Add(time.Duration(i)),
			Module:   fmt.Sprintf("module%d", i),
			Location: fmt.Sprintf("filename%d:%d", i, i),
			Level:    loggo.Level(i),
			Message:  fmt.Sprintf("message%d", i),
		})
	}
}

func (s *bufferedLogWriterSuite) TestLimiting(c *tc.C) {
	write := func(msgNum int) {
		s.writer.Write(
			loggo.Entry{
				Level:     loggo.INFO,
				Module:    "module",
				Filename:  "filename",
				Line:      42,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("log%d", msgNum),
			})
	}

	expect := func(msgNum, dropped int) {
		rec := s.receiveOne(c)
		c.Assert(rec.Message, tc.Equals, fmt.Sprintf("log%d", msgNum))
		c.Assert(rec.DroppedAfter, tc.Equals, dropped)
	}

	// Write more logs than the buffer allows.
	for i := 0; i < maxLen+3; i++ {
		write(i)
	}

	// Read some logs from the writer.

	// Even though logs have been dropped, log 0 is still seen
	// first. This is useful because means the time range for dropped
	// logs can be observed.
	expect(0, 2) // logs 1 and 2 dropped here
	expect(3, 0)
	expect(4, 0)

	// Now write more logs, again exceeding the limit.
	for i := maxLen + 3; i < maxLen+3+maxLen; i++ {
		write(i)
	}

	// Read all the remaining logs off.
	expect(5, 3) // logs 6, 7 and 8 logs dropped here
	for i := 9; i < maxLen+3+maxLen; i++ {
		expect(i, 0)
	}

	logsendertest.ExpectLogStats(c, s.writer, logsender.LogStats{
		Enqueued: maxLen*2 + 3,
		Sent:     maxLen*2 + 3 - 5,
		Dropped:  5,
	})
}

func (s *bufferedLogWriterSuite) TestClose(c *tc.C) {
	s.writer.Close()
	s.shouldClose = false // Prevent the usual teardown (calling Close twice will panic)

	// Output channel closing means the bufferedLogWriterSuite loop
	// has finished.
	select {
	case _, ok := <-s.writer.Logs():
		c.Assert(ok, tc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for output channel to close")
	}

	// Further Write attempts should fail.
	c.Assert(func() { s.writeAndReceive(c) }, tc.PanicMatches, ".*send on closed channel")
}

func (s *bufferedLogWriterSuite) TestInstallBufferedLogWriter(c *tc.C) {
	bufferedLogger, err := logsender.InstallBufferedLogWriter(loggo.DefaultContext(), 10)
	c.Assert(err, tc.ErrorIsNil)
	defer logsender.UninstallBufferedLogWriter()

	logger := internallogger.GetLogger("bufferedLogWriter-test")

	for i := 0; i < 5; i++ {
		logger.Infof(context.TODO(), "%d", i)
	}

	logsCh := bufferedLogger.Logs()
	for i := 0; i < 5; i++ {
		select {
		case rec := <-logsCh:
			c.Assert(rec.Message, tc.Equals, strconv.Itoa(i))
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for logs")
		}
	}
}

func (s *bufferedLogWriterSuite) TestUninstallBufferedLogWriter(c *tc.C) {
	_, err := logsender.InstallBufferedLogWriter(loggo.DefaultContext(), 10)
	c.Assert(err, tc.ErrorIsNil)

	err = logsender.UninstallBufferedLogWriter()
	c.Assert(err, tc.ErrorIsNil)

	// Second uninstall attempt should fail
	err = logsender.UninstallBufferedLogWriter()
	c.Assert(err, tc.ErrorMatches, "failed to uninstall log buffering: .+")
}

func (s *bufferedLogWriterSuite) writeAndReceive(c *tc.C) {
	now := time.Now()
	s.writer.Write(
		loggo.Entry{
			Level:     loggo.INFO,
			Module:    "module",
			Filename:  "filename",
			Line:      99,
			Timestamp: now,
			Message:   "message",
		})
	c.Assert(*s.receiveOne(c), tc.DeepEquals, logsender.LogRecord{
		Time:     now,
		Module:   "module",
		Location: "filename:99",
		Level:    loggo.INFO,
		Message:  "message",
	})
}

func (s *bufferedLogWriterSuite) receiveOne(c *tc.C) *logsender.LogRecord {
	select {
	case rec := <-s.writer.Logs():
		return rec
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for log record")
	}
	panic("should never get here")
}
