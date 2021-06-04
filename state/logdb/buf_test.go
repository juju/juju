// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logdb_test

import (
	"errors"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/logdb"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&BufferedLoggerSuite{})

type BufferedLoggerSuite struct {
	testing.IsolationSuite
}

func (s *BufferedLoggerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *BufferedLoggerSuite) waitFlush(c *gc.C, mock *mockLogger) []state.LogRecord {
	select {
	case records := <-mock.called:
		return records
	case <-time.After(coretesting.LongWait):
	}
	c.Fatal("timed out waiting for logs to be flushed")
	panic("unreachable")
}

func (s *BufferedLoggerSuite) assertNoFlush(c *gc.C, mock *mockLogger, clock *testclock.Clock) {
	err := clock.WaitAdvance(0, 0, 0) // There should be no active timers
	c.Assert(err, jc.ErrorIsNil)
	select {
	case records := <-mock.called:
		c.Fatalf("unexpected log records: %v", records)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *BufferedLoggerSuite) TestLogFlushes(c *gc.C) {
	const bufsz = 3
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	b := logdb.NewBufferedLogger(&mock, bufsz, time.Minute, clock)
	in := []state.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}, {
		Entity:  "not-a-tag",
		Message: "baz",
	}}

	err := b.Log(in[:2])
	c.Assert(err, jc.ErrorIsNil)
	mock.CheckNoCalls(c)

	err = b.Log(in[2:])
	c.Assert(err, jc.ErrorIsNil)
	mock.CheckCalls(c, []testing.StubCall{
		{"Log", []interface{}{in}},
	})

	err = clock.WaitAdvance(0, coretesting.LongWait, 0)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoFlush(c, &mock, clock)
}

func (s *BufferedLoggerSuite) TestLogFlushesMultiple(c *gc.C) {
	const bufsz = 1
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	b := logdb.NewBufferedLogger(&mock, bufsz, time.Minute, clock)
	in := []state.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}, {
		Entity:  "not-a-tag",
		Message: "baz",
	}}

	err := b.Log(in)
	c.Assert(err, jc.ErrorIsNil)
	mock.CheckCalls(c, []testing.StubCall{
		{"Log", []interface{}{in[:1]}},
		{"Log", []interface{}{in[1:2]}},
		{"Log", []interface{}{in[2:]}},
	})
}

func (s *BufferedLoggerSuite) TestTimerFlushes(c *gc.C) {
	const bufsz = 10
	const flushInterval = time.Minute
	mock := mockLogger{called: make(chan []state.LogRecord)}
	clock := testclock.NewClock(time.Time{})

	b := logdb.NewBufferedLogger(&mock, bufsz, flushInterval, clock)
	in := []state.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}}

	err := b.Log(in[:1])
	c.Assert(err, jc.ErrorIsNil)
	mock.CheckNoCalls(c)

	// Advance, but not far enough to trigger the flush.
	clock.WaitAdvance(30*time.Second, coretesting.LongWait, 1)
	mock.CheckNoCalls(c)

	// Log again; the timer should not have been reset.
	err = b.Log(in[1:])
	c.Assert(err, jc.ErrorIsNil)
	mock.CheckNoCalls(c)

	// Advance to to the flush interval.
	clock.Advance(30 * time.Second)
	c.Assert(s.waitFlush(c, &mock), jc.DeepEquals, in)
	mock.CheckCalls(c, []testing.StubCall{
		{"Log", []interface{}{in}},
	})
	s.assertNoFlush(c, &mock, clock)
	mock.ResetCalls()

	// Logging again, the timer resets to the time at which
	// the new log records are inserted.
	err = b.Log(in)
	c.Assert(err, jc.ErrorIsNil)
	clock.WaitAdvance(59*time.Second, coretesting.LongWait, 1)
	mock.CheckNoCalls(c)
	clock.Advance(1 * time.Second)
	c.Assert(s.waitFlush(c, &mock), jc.DeepEquals, in)
	mock.CheckCalls(c, []testing.StubCall{
		{"Log", []interface{}{in}},
	})
	s.assertNoFlush(c, &mock, clock)
}

func (s *BufferedLoggerSuite) TestLogOverCapacity(c *gc.C) {
	const bufsz = 2
	const flushInterval = time.Minute
	mock := mockLogger{called: make(chan []state.LogRecord, 1)}
	clock := testclock.NewClock(time.Time{})

	// The buffer has a capacity of 2, so writing 3 logs will
	// cause 2 to be flushed, with 1 remaining in the buffer
	// until the timer triggers.
	b := logdb.NewBufferedLogger(&mock, bufsz, flushInterval, clock)
	in := []state.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}, {
		Entity:  "not-a-tag",
		Message: "baz",
	}}

	err := b.Log(in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.waitFlush(c, &mock), jc.DeepEquals, in[:bufsz])

	clock.WaitAdvance(time.Minute, coretesting.LongWait, 1)
	c.Assert(s.waitFlush(c, &mock), jc.DeepEquals, in[bufsz:])

	mock.CheckCalls(c, []testing.StubCall{
		{"Log", []interface{}{in[:bufsz]}},
		{"Log", []interface{}{in[bufsz:]}},
	})
}

func (s *BufferedLoggerSuite) TestFlushNothing(c *gc.C) {
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	b := logdb.NewBufferedLogger(&mock, 1, time.Minute, clock)
	err := b.Flush()
	c.Assert(err, jc.ErrorIsNil)
	mock.CheckNoCalls(c)
}

func (s *BufferedLoggerSuite) TestFlushReportsError(c *gc.C) {
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	mock.SetErrors(errors.New("nope"))
	b := logdb.NewBufferedLogger(&mock, 2, time.Minute, clock)
	err := b.Log([]state.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}})
	c.Assert(err, jc.ErrorIsNil)
	err = b.Flush()
	c.Assert(err, gc.ErrorMatches, "nope")
}

func (s *BufferedLoggerSuite) TestLogReportsError(c *gc.C) {
	mock := mockLogger{}
	clock := testclock.NewClock(time.Time{})
	mock.SetErrors(errors.New("nope"))
	b := logdb.NewBufferedLogger(&mock, 1, time.Minute, clock)
	err := b.Log([]state.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}})
	c.Assert(err, gc.ErrorMatches, "nope")
}

type mockLogger struct {
	testing.Stub
	called chan []state.LogRecord
}

func (m *mockLogger) Log(in []state.LogRecord) error {
	incopy := make([]state.LogRecord, len(in))
	copy(incopy, in)
	m.MethodCall(m, "Log", incopy)
	if m.called != nil {
		m.called <- incopy
	}
	return m.NextErr()
}
