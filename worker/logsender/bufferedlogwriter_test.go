// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package logsender_test

import (
	"fmt"
	"strconv"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/logsender"
)

const maxLen = 6

type bufferedLogWriterSuite struct {
	coretesting.BaseSuite
	writer      *logsender.BufferedLogWriter
	shouldClose bool
}

var _ = gc.Suite(&bufferedLogWriterSuite{})

func (s *bufferedLogWriterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.writer = logsender.NewBufferedLogWriter(maxLen)
	s.shouldClose = true
}

func (s *bufferedLogWriterSuite) TearDownTest(c *gc.C) {
	if s.shouldClose {
		s.writer.Close()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *bufferedLogWriterSuite) TestOne(c *gc.C) {
	s.writeAndReceive(c)
}

func (s *bufferedLogWriterSuite) TestMultiple(c *gc.C) {
	for i := 0; i < 10; i++ {
		s.writeAndReceive(c)
	}
}

func (s *bufferedLogWriterSuite) TestBuffering(c *gc.C) {
	// Write several log message before attempting to read them out.
	const numMessages = 5
	now := time.Now()
	for i := 0; i < numMessages; i++ {
		s.writer.Write(
			loggo.Level(i),
			fmt.Sprintf("module%d", i),
			fmt.Sprintf("filename%d", i),
			i, // line number
			now.Add(time.Duration(i)),
			fmt.Sprintf("message%d", i),
		)
	}

	for i := 0; i < numMessages; i++ {
		c.Assert(*s.receiveOne(c), gc.DeepEquals, logsender.LogRecord{
			Time:     now.Add(time.Duration(i)),
			Module:   fmt.Sprintf("module%d", i),
			Location: fmt.Sprintf("filename%d:%d", i, i),
			Level:    loggo.Level(i),
			Message:  fmt.Sprintf("message%d", i),
		})
	}
}

func (s *bufferedLogWriterSuite) TestLimiting(c *gc.C) {
	write := func(msgNum int) {
		s.writer.Write(loggo.INFO, "module", "filename", 42, time.Now(), fmt.Sprintf("log%d", msgNum))
	}

	expect := func(msgNum, dropped int) {
		rec := s.receiveOne(c)
		c.Assert(rec.Message, gc.Equals, fmt.Sprintf("log%d", msgNum))
		c.Assert(rec.DroppedAfter, gc.Equals, dropped)
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
}

func (s *bufferedLogWriterSuite) TestClose(c *gc.C) {
	s.writer.Close()
	s.shouldClose = false // Prevent the usual teardown (calling Close twice will panic)

	// Output channel closing means the bufferedLogWriterSuite loop
	// has finished.
	select {
	case _, ok := <-s.writer.Logs():
		c.Assert(ok, jc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for output channel to close")
	}

	// Further Write attempts should fail.
	c.Assert(func() { s.writeAndReceive(c) }, gc.PanicMatches, ".*send on closed channel")
}

func (s *bufferedLogWriterSuite) TestInstallBufferedLogWriter(c *gc.C) {
	s.SetFeatureFlags("db-log")

	logsCh, err := logsender.InstallBufferedLogWriter(10)
	c.Assert(err, jc.ErrorIsNil)
	defer logsender.UninstallBufferedLogWriter()

	logger := loggo.GetLogger("bufferedLogWriter-test")

	for i := 0; i < 5; i++ {
		logger.Infof("%d", i)
	}

	for i := 0; i < 5; i++ {
		select {
		case rec := <-logsCh:
			c.Assert(rec.Message, gc.Equals, strconv.Itoa(i))
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for logs")
		}
	}
}

func (s *bufferedLogWriterSuite) TestUninstallBufferedLogWriter(c *gc.C) {
	s.SetFeatureFlags("db-log")

	_, err := logsender.InstallBufferedLogWriter(10)
	c.Assert(err, jc.ErrorIsNil)

	err = logsender.UninstallBufferedLogWriter()
	c.Assert(err, jc.ErrorIsNil)

	// Second uninstall attempt should fail
	err = logsender.UninstallBufferedLogWriter()
	c.Assert(err, gc.ErrorMatches, "failed to uninstall log buffering: .+")
}

func (s *bufferedLogWriterSuite) TestInstallBufferedLogWriterNoFeatureFlag(c *gc.C) {
	logsCh, err := logsender.InstallBufferedLogWriter(10)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(logsCh, gc.IsNil)
}

func (s *bufferedLogWriterSuite) TestUninstallBufferedLogWriterNoFeatureFlag(c *gc.C) {
	err := logsender.UninstallBufferedLogWriter()
	// With the feature flag, uninstalling without first installing
	// would result in an error.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bufferedLogWriterSuite) writeAndReceive(c *gc.C) {
	now := time.Now()
	s.writer.Write(loggo.INFO, "module", "filename", 99, now, "message")
	c.Assert(*s.receiveOne(c), gc.DeepEquals, logsender.LogRecord{
		Time:     now,
		Module:   "module",
		Location: "filename:99",
		Level:    loggo.INFO,
		Message:  "message",
	})
}

func (s *bufferedLogWriterSuite) receiveOne(c *gc.C) *logsender.LogRecord {
	select {
	case rec := <-s.writer.Logs():
		return rec
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for log record")
	}
	panic("should never get here")
}
