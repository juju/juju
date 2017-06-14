package logdb_test

import (
	"errors"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/logdb"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&BufferedLoggerSuite{})

type BufferedLoggerSuite struct {
	testing.IsolationSuite

	mock  mockLogger
	clock *testing.Clock
}

func (s *BufferedLoggerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mock = mockLogger{}
	s.clock = testing.NewClock(time.Time{})
}

func (s *BufferedLoggerSuite) TestLogFlushes(c *gc.C) {
	const bufsz = 3
	b := logdb.NewBufferedLogger(&s.mock, bufsz, time.Minute, s.clock)
	in := []state.LogRecord{{
		Entity:  names.NewMachineTag("0"),
		Message: "foo",
	}, {
		Entity:  names.NewMachineTag("0"),
		Message: "bar",
	}, {
		Entity:  names.NewMachineTag("0"),
		Message: "baz",
	}}

	err := b.Log(in[:2])
	c.Assert(err, jc.ErrorIsNil)
	s.mock.CheckNoCalls(c)

	err = b.Log(in[2:])
	c.Assert(err, jc.ErrorIsNil)
	s.mock.CheckCalls(c, []testing.StubCall{
		{"Log", []interface{}{in}},
	})
}

func (s *BufferedLoggerSuite) TestLogFlushesMultiple(c *gc.C) {
	const bufsz = 1
	b := logdb.NewBufferedLogger(&s.mock, bufsz, time.Minute, s.clock)
	in := []state.LogRecord{{
		Entity:  names.NewMachineTag("0"),
		Message: "foo",
	}, {
		Entity:  names.NewMachineTag("0"),
		Message: "bar",
	}, {
		Entity:  names.NewMachineTag("0"),
		Message: "baz",
	}}

	err := b.Log(in)
	c.Assert(err, jc.ErrorIsNil)
	s.mock.CheckCalls(c, []testing.StubCall{
		{"Log", []interface{}{in[:1]}},
		{"Log", []interface{}{in[1:2]}},
		{"Log", []interface{}{in[2:]}},
	})
}

func (s *BufferedLoggerSuite) TestTimerFlushes(c *gc.C) {
	const bufsz = 10
	const flushInterval = time.Minute
	s.mock.called = make(chan []state.LogRecord)

	b := logdb.NewBufferedLogger(&s.mock, bufsz, flushInterval, s.clock)
	in := []state.LogRecord{{
		Entity:  names.NewMachineTag("0"),
		Message: "foo",
	}, {
		Entity:  names.NewMachineTag("0"),
		Message: "bar",
	}}

	err := b.Log(in[:1])
	c.Assert(err, jc.ErrorIsNil)
	s.mock.CheckNoCalls(c)

	// Advance, but not far enough to trigger the flush.
	s.clock.WaitAdvance(30*time.Second, coretesting.LongWait, 1)
	s.mock.CheckNoCalls(c)

	// Log again; the timer should not have been reset.
	err = b.Log(in[1:])
	s.mock.CheckNoCalls(c)

	// Advance to to the flush interval.
	s.clock.Advance(30 * time.Second)
	select {
	case records := <-s.mock.called:
		c.Assert(records, jc.DeepEquals, in)
		s.mock.CheckCalls(c, []testing.StubCall{
			{"Log", []interface{}{in}},
		})
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for logs to be flushed")
	}
}

func (s *BufferedLoggerSuite) TestFlushNothing(c *gc.C) {
	b := logdb.NewBufferedLogger(&s.mock, 1, time.Minute, s.clock)
	err := b.Flush()
	c.Assert(err, jc.ErrorIsNil)
	s.mock.CheckNoCalls(c)
}

func (s *BufferedLoggerSuite) TestFlushReportsError(c *gc.C) {
	s.mock.SetErrors(errors.New("nope"))
	b := logdb.NewBufferedLogger(&s.mock, 2, time.Minute, s.clock)
	err := b.Log([]state.LogRecord{{
		Entity:  names.NewMachineTag("0"),
		Message: "foo",
	}})
	c.Assert(err, jc.ErrorIsNil)
	err = b.Flush()
	c.Assert(err, gc.ErrorMatches, "nope")
}

func (s *BufferedLoggerSuite) TestLogReportsError(c *gc.C) {
	s.mock.SetErrors(errors.New("nope"))
	b := logdb.NewBufferedLogger(&s.mock, 1, time.Minute, s.clock)
	err := b.Log([]state.LogRecord{{
		Entity:  names.NewMachineTag("0"),
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
