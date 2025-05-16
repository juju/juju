// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"fmt"
	"math/rand"
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/kr/pretty"

	corelogger "github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type BufferedLogWriterSuite struct {
	testhelpers.IsolationSuite
}

func TestBufferedLogWriterSuite(t *stdtesting.T) { tc.Run(t, &BufferedLogWriterSuite{}) }
func (s *BufferedLogWriterSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *BufferedLogWriterSuite) waitFlush(c *tc.C, mock *mockLogRecorder) []corelogger.LogRecord {
	select {
	case records := <-mock.called:
		c.Log("REC: " + pretty.Sprint(records))
		return records
	case <-time.After(coretesting.LongWait):
	}
	c.Fatal("timed out waiting for logs to be flushed")
	panic("unreachable")
}

func (s *BufferedLogWriterSuite) assertNoFlush(c *tc.C, mock *mockLogRecorder, clock *testclock.Clock) {
	err := clock.WaitAdvance(0, 0, 0) // There should be no active timers
	c.Assert(err, tc.ErrorIsNil)
	select {
	case records := <-mock.called:
		c.Fatalf("unexpected log records: %v", records)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *BufferedLogWriterSuite) TestLogFlushes(c *tc.C) {
	const bufsz = 3
	mock := mockLogRecorder{}
	clock := testclock.NewClock(time.Time{})
	b := corelogger.NewBufferedLogWriter(&mock, bufsz, time.Minute, clock)
	in := []corelogger.LogRecord{{
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
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckNoCalls(c)

	err = b.Log(in[2:])
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "Log", Args: []any{in}},
	})

	err = clock.WaitAdvance(0, coretesting.LongWait, 0)
	c.Assert(err, tc.ErrorIsNil)
	s.assertNoFlush(c, &mock, clock)
}

func (s *BufferedLogWriterSuite) TestLogFlushesMultiple(c *tc.C) {
	const bufsz = 1
	mock := mockLogRecorder{}
	clock := testclock.NewClock(time.Time{})
	b := corelogger.NewBufferedLogWriter(&mock, bufsz, time.Minute, clock)
	in := []corelogger.LogRecord{{
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
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "Log", Args: []any{in[:1]}},
		{FuncName: "Log", Args: []any{in[1:2]}},
		{FuncName: "Log", Args: []any{in[2:]}},
	})
}

func (s *BufferedLogWriterSuite) TestTimerFlushes(c *tc.C) {
	const bufsz = 10
	const flushInterval = time.Minute
	mock := mockLogRecorder{called: make(chan []corelogger.LogRecord)}
	clock := testclock.NewClock(time.Time{})

	b := corelogger.NewBufferedLogWriter(&mock, bufsz, flushInterval, clock)
	in := []corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}, {
		Entity:  "not-a-tag",
		Message: "bar",
	}}

	err := b.Log(in[:1])
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckNoCalls(c)

	// Advance, but not far enough to trigger the flush.
	clock.WaitAdvance(30*time.Second, coretesting.LongWait, 1)
	mock.CheckNoCalls(c)

	// Log again; the timer should not have been reset.
	err = b.Log(in[1:])
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckNoCalls(c)

	// Advance to to the flush interval.
	clock.Advance(30 * time.Second)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, in)
	mock.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "Log", Args: []any{in}},
	})
	s.assertNoFlush(c, &mock, clock)
	mock.ResetCalls()

	// Logging again, the timer resets to the time at which
	// the new log records are inserted.
	err = b.Log(in)
	c.Assert(err, tc.ErrorIsNil)
	clock.WaitAdvance(59*time.Second, coretesting.LongWait, 1)
	mock.CheckNoCalls(c)
	clock.Advance(1 * time.Second)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, in)
	mock.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "Log", Args: []any{in}},
	})
	s.assertNoFlush(c, &mock, clock)
}

func (s *BufferedLogWriterSuite) TestLogOverCapacity(c *tc.C) {
	const bufsz = 2
	const flushInterval = time.Minute
	mock := mockLogRecorder{called: make(chan []corelogger.LogRecord, 1)}
	clock := testclock.NewClock(time.Time{})

	// The buffer has a capacity of 2, so writing 3 logs will
	// cause 2 to be flushed, with 1 remaining in the buffer
	// until the timer triggers.
	b := corelogger.NewBufferedLogWriter(&mock, bufsz, flushInterval, clock)
	in := []corelogger.LogRecord{{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, in[:bufsz])

	clock.WaitAdvance(time.Minute, coretesting.LongWait, 1)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, in[bufsz:])

	mock.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "Log", Args: []any{in[:bufsz]}},
		{FuncName: "Log", Args: []any{in[bufsz:]}},
	})
}

func (s *BufferedLogWriterSuite) TestFlushSorts(c *tc.C) {
	const bufsz = 2
	const flushInterval = time.Minute
	mock := mockLogRecorder{called: make(chan []corelogger.LogRecord, 1)}
	clock := testclock.NewClock(time.Time{})

	// The buffer has a capacity of 2, so writing 3 logs will
	// cause 2 to be flushed, with 1 remaining in the buffer
	// until the timer triggers.
	now := time.Now()
	r1 := corelogger.LogRecord{
		Time:    now.Add(2 * time.Second),
		Entity:  "not-a-tag",
		Message: "foo",
	}
	r2 := corelogger.LogRecord{
		Time:    now.Add(time.Second),
		Entity:  "not-a-tag",
		Message: "bar",
	}
	r3 := corelogger.LogRecord{
		Time:    now,
		Entity:  "not-a-tag",
		Message: "baz",
	}
	b := corelogger.NewBufferedLogWriter(&mock, bufsz, flushInterval, clock)
	in := []corelogger.LogRecord{r1, r2, r3}

	err := b.Log(in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, []corelogger.LogRecord{r3, r2})

	clock.WaitAdvance(time.Minute, coretesting.LongWait, 1)
	c.Assert(s.waitFlush(c, &mock), tc.DeepEquals, []corelogger.LogRecord{r1})

	mock.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "Log", Args: []any{[]corelogger.LogRecord{r3, r2}}},
		{FuncName: "Log", Args: []any{[]corelogger.LogRecord{r1}}},
	})
}

func (s *BufferedLogWriterSuite) TestInsertSorts(c *tc.C) {
	const bufsz = 10
	const flushInterval = time.Minute
	mock := mockLogRecorder{called: make(chan []corelogger.LogRecord, 1)}
	clock := testclock.NewDilatedWallClock(time.Millisecond)

	now := time.Now()
	initial := make([]corelogger.LogRecord, 5)
	for i := 0; i < 5; i++ {
		d := rand.Int63n(int64(20))
		r := corelogger.LogRecord{
			Time:    now.Add(time.Millisecond * time.Duration(d)),
			Entity:  "not-a-tag",
			Message: fmt.Sprintf("foo-%d", i),
		}
		initial[i] = r
	}
	b := corelogger.NewBufferedLogWriter(&mock, bufsz, flushInterval, clock)

	err := b.Log(initial)
	c.Assert(err, tc.ErrorIsNil)

	in := make([]corelogger.LogRecord, 5)
	for i := 0; i < 5; i++ {
		d := rand.Int63n(int64(20))
		r := corelogger.LogRecord{
			Time:    now.Add(time.Millisecond * time.Duration(d)),
			Entity:  "not-a-tag",
			Message: fmt.Sprintf("foo-%d", 5+i),
		}
		in[i] = r
	}

	err = b.Log(in)
	c.Assert(err, tc.ErrorIsNil)

	clock.Advance(time.Minute)
	records := s.waitFlush(c, &mock)
	c.Assert(records, tc.HasLen, 10)

	lastTime := now
	for _, rec := range records {
		c.Assert(!rec.Time.Before(lastTime), tc.IsTrue)
		lastTime = rec.Time
	}
}

func (s *BufferedLogWriterSuite) TestFlushNothing(c *tc.C) {
	mock := mockLogRecorder{}
	clock := testclock.NewClock(time.Time{})
	b := corelogger.NewBufferedLogWriter(&mock, 1, time.Minute, clock)
	err := b.Flush()
	c.Assert(err, tc.ErrorIsNil)
	mock.CheckNoCalls(c)
}

func (s *BufferedLogWriterSuite) TestFlushReportsError(c *tc.C) {
	mock := mockLogRecorder{}
	clock := testclock.NewClock(time.Time{})
	mock.SetErrors(errors.New("nope"))
	b := corelogger.NewBufferedLogWriter(&mock, 2, time.Minute, clock)
	err := b.Log([]corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}})
	c.Assert(err, tc.ErrorIsNil)
	err = b.Flush()
	c.Assert(err, tc.ErrorMatches, "nope")
}

func (s *BufferedLogWriterSuite) TestLogReportsError(c *tc.C) {
	mock := mockLogRecorder{}
	clock := testclock.NewClock(time.Time{})
	mock.SetErrors(errors.New("nope"))
	b := corelogger.NewBufferedLogWriter(&mock, 1, time.Minute, clock)
	err := b.Log([]corelogger.LogRecord{{
		Entity:  "not-a-tag",
		Message: "foo",
	}})
	c.Assert(err, tc.ErrorMatches, "nope")
}

type mockLogRecorder struct {
	testhelpers.Stub
	called chan []corelogger.LogRecord
}

func (m *mockLogRecorder) Log(in []corelogger.LogRecord) error {
	incopy := make([]corelogger.LogRecord, len(in))
	copy(incopy, in)
	m.MethodCall(m, "Log", incopy)
	if m.called != nil {
		m.called <- incopy
	}
	return m.NextErr()
}
