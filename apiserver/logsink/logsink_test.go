// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/utils/v4"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/logsink/mocks"
	"github.com/juju/juju/apiserver/websocket/websockettest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

var shortAttempt = &utils.AttemptStrategy{
	Total: coretesting.ShortWait,
	Delay: 10 * time.Millisecond,
}

var longAttempt = &utils.AttemptStrategy{
	Total: coretesting.LongWait,
	Delay: 10 * time.Millisecond,
}

type logsinkSuite struct {
	testing.IsolationSuite

	abort chan struct{}

	stub    *testing.Stub
	written chan params.LogRecord

	lastStack []byte
	stackMu   sync.Mutex
}

var _ = tc.Suite(&logsinkSuite{})

func (s *logsinkSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.abort = make(chan struct{})
	s.written = make(chan params.LogRecord, 1)
	s.stub = &testing.Stub{}
	s.stackMu.Lock()
	s.lastStack = nil
	s.stackMu.Unlock()
}

func (s *logsinkSuite) dialWebsocket(c *tc.C, srv *httptest.Server) *websocket.Conn {
	u, err := url.Parse(srv.URL)
	c.Assert(err, tc.ErrorIsNil)
	u.Scheme = "ws"
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { conn.Close() })
	return conn
}

func (s *logsinkSuite) TestSuccess(c *tc.C) {
	srv, finish := s.createServer(c)
	defer finish()

	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	record := params.LogRecord{
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	}
	err := conn.WriteJSON(&record)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case written, ok := <-s.written:
		c.Assert(ok, tc.IsTrue)
		c.Assert(written, tc.DeepEquals, record)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for log record to be written")
	}
	select {
	case <-s.written:
		c.Fatal("unexpected log record")
	case <-time.After(coretesting.ShortWait):
	}
	s.stub.CheckCallNames(c, "Open", "WriteLog")

	s.stackMu.Lock()
	if s.lastStack != nil {
		c.Logf("last Close call stack: \n%s", string(s.lastStack))
	}
	s.stackMu.Unlock()

	err = conn.Close()
	c.Assert(err, tc.ErrorIsNil)
	for a := longAttempt.Start(); a.Next(); {
		if len(s.stub.Calls()) == 3 {
			break
		}
	}
	s.stub.CheckCallNames(c, "Open", "WriteLog")
}

func (s *logsinkSuite) TestLogMessages(c *tc.C) {
	srv, finish := s.createServer(c)
	defer finish()

	var logs loggo.TestWriter
	writer := loggo.NewMinimumLevelWriter(&logs, loggo.INFO)
	c.Assert(loggo.RegisterWriter("logsink-tests", writer), tc.ErrorIsNil)

	// Open, then close connection.
	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONInitialErrorNil(c, conn)
	err := conn.Close()
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that no error is logged when the connection is closed normally.
	for a := shortAttempt.Start(); a.Next(); {
		for _, log := range logs.Log() {
			c.Assert(log.Level, tc.LessThan, loggo.ERROR, tc.Commentf("log: %#v", log))
		}
	}
}

func (s *logsinkSuite) TestSuccessWithLabels(c *tc.C) {
	srv, finish := s.createServer(c)
	defer finish()

	var logs loggo.TestWriter
	writer := loggo.NewMinimumLevelWriter(&logs, loggo.INFO)
	c.Assert(loggo.RegisterWriter("logsink-tests", writer), tc.ErrorIsNil)

	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	records := []params.LogRecord{{
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	}, {
		Time:     t0,
		Module:   "some.where.else",
		Location: "bar.go:122",
		Level:    loggo.INFO.String(),
		Message:  "all is nice",
		Entity:   "entity.name",
		Labels:   map[string]string{"foo": "bar"},
	}, {
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:33",
		Level:    loggo.INFO.String(),
		Message:  "all is fine",
	}}
	for _, record := range records {
		err := conn.WriteJSON(&record)
		c.Assert(err, tc.ErrorIsNil)

		select {
		case written, ok := <-s.written:
			c.Assert(ok, tc.IsTrue)
			c.Assert(written, tc.DeepEquals, record)
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for log record to be written")
		}
		select {
		case <-s.written:
			c.Fatal("unexpected log record")
		case <-time.After(coretesting.ShortWait):
		}

	}

	s.stub.CheckCallNames(c, "Open", "WriteLog", "WriteLog", "WriteLog")

	s.stackMu.Lock()
	if s.lastStack != nil {
		c.Logf("last Close call stack: \n%s", string(s.lastStack))
	}
	s.stackMu.Unlock()

	err := conn.Close()
	c.Assert(err, tc.ErrorIsNil)
	for a := longAttempt.Start(); a.Next(); {
		if len(s.stub.Calls()) == 3 {
			break
		}
	}
	s.stub.CheckCallNames(c, "Open", "WriteLog", "WriteLog", "WriteLog")
}

func (s *logsinkSuite) TestLogOpenFails(c *tc.C) {
	srv, finish := s.createServer(c)
	defer finish()

	s.stub.SetErrors(errors.New("rats"))
	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONError(c, conn, "rats")
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *logsinkSuite) TestLogWriteFails(c *tc.C) {
	srv, finish := s.createServer(c)
	defer finish()

	s.stub.SetErrors(nil, errors.New("cannae write"))
	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	err := conn.WriteJSON(&params.LogRecord{})
	c.Assert(err, tc.ErrorIsNil)

	websockettest.AssertJSONError(c, conn, "cannae write")
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *logsinkSuite) TestReceiveErrorBreaksConn(c *tc.C) {
	srv, finish := s.createServer(c)
	defer finish()

	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	// The logsink handler expects JSON messages. Send some
	// junk to verify that the server closes the connection.
	err := conn.WriteMessage(websocket.TextMessage, []byte("junk!"))
	c.Assert(err, tc.ErrorIsNil)

	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *logsinkSuite) TestRateLimit(c *tc.C) {
	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	metricsCollector, finish := createMockMetrics(c, modelUUID.String())
	defer finish()

	testClock := testclock.NewClock(time.Time{})
	srv := httptest.NewServer(logsink.NewHTTPHandler(
		func(req *http.Request) (logsink.LogWriter, error) {
			s.stub.AddCall("Open")
			return &mockLogWriter{
				s.stub,
				s.written,
				nil,
			}, s.stub.NextErr()
		},
		s.abort,
		&logsink.RateLimitConfig{
			Burst:  2,
			Refill: time.Second,
			Clock:  testClock,
		},
		metricsCollector,
		modelUUID.String(),
	))
	defer srv.Close()

	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	record := params.LogRecord{
		Time:     time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC),
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	}
	for i := 0; i < 4; i++ {
		err := conn.WriteJSON(&record)
		c.Assert(err, tc.ErrorIsNil)
	}

	expectRecord := func() {
		select {
		case written, ok := <-s.written:
			c.Assert(ok, tc.IsTrue)
			c.Assert(written, tc.DeepEquals, record)
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for log record to be written")
		}
	}
	expectNoRecord := func() {
		select {
		case <-s.written:
			c.Fatal("unexpected log record")
		case <-time.After(coretesting.ShortWait):
		}
	}

	// There should be 2 records received immediately,
	// and then rate-limiting should kick in.
	expectRecord()
	expectRecord()
	expectNoRecord()
	testClock.WaitAdvance(time.Second, coretesting.LongWait, 1)
	expectRecord()
	expectNoRecord()
	testClock.WaitAdvance(time.Second, coretesting.LongWait, 1)
	expectRecord()
	expectNoRecord()
}

func (s *logsinkSuite) TestReceiverStopsWhenAsked(c *tc.C) {
	myStopCh := make(chan struct{})

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	metricsCollector, finish := createMockMetrics(c, modelUUID.String())
	defer finish()

	handler := logsink.NewHTTPHandlerForTest(
		func(req *http.Request) (logsink.LogWriter, error) {
			s.stub.AddCall("Open")
			return &slowWriteCloser{}, s.stub.NextErr()
		},
		s.abort,
		nil,
		metricsCollector,
		modelUUID.String(),
		func() (chan struct{}, func()) {
			return myStopCh, func() {}
		},
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	close(myStopCh)

	// Send 2 log messages so we're sure the receiver gets a chance to
	// go down the stop channel leg, since the writes are slow.
	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	record := params.LogRecord{
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	}
	err = conn.WriteJSON(&record)
	c.Assert(err, tc.ErrorIsNil)
	// The second write might error (if the receiver stopped after the
	// first message).
	_ = conn.WriteJSON(&record)

	for a := longAttempt.Start(); a.Next(); {
		if logsink.ReceiverStopped(c, handler) {
			break
		}
	}
	c.Assert(logsink.ReceiverStopped(c, handler), tc.Equals, true)

	err = conn.Close()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *logsinkSuite) TestHandlerClosesStopChannel(c *tc.C) {
	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	metricsCollector, finish := createMockMetrics(c, modelUUID.String())
	defer finish()

	var stub testing.Stub
	handler := logsink.NewHTTPHandlerForTest(
		func(req *http.Request) (logsink.LogWriter, error) {
			return &mockLogWriter{
				s.stub,
				s.written,
				nil,
			}, s.stub.NextErr()
		},
		s.abort,
		nil,
		metricsCollector,
		modelUUID.String(),
		func() (chan struct{}, func()) {
			ch := make(chan struct{})
			return ch, func() {
				stub.AddCall("close stop channel")
				close(ch)
			}
		},
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()
	conn := s.dialWebsocket(c, srv)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	t0 := time.Date(2015, time.June, 1, 23, 2, 1, 0, time.UTC)
	record := params.LogRecord{
		Time:     t0,
		Module:   "some.where",
		Location: "foo.go:42",
		Level:    loggo.INFO.String(),
		Message:  "all is well",
	}
	err = conn.WriteJSON(&record)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case written, ok := <-s.written:
		c.Assert(ok, tc.IsTrue)
		c.Assert(written, tc.DeepEquals, record)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for log record to be written")
	}

	err = conn.Close()
	c.Assert(err, tc.ErrorIsNil)
	for a := longAttempt.Start(); a.Next(); {
		if len(stub.Calls()) == 1 {
			break
		}
	}
	stub.CheckCallNames(c, "close stop channel")
}

func (s *logsinkSuite) createServer(c *tc.C) (*httptest.Server, func()) {
	recordStack := func() {
		s.stackMu.Lock()
		defer s.stackMu.Unlock()
		s.lastStack = debug.Stack()
	}

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	metricsCollector, finish := createMockMetrics(c, modelUUID.String())

	srv := httptest.NewServer(logsink.NewHTTPHandler(
		func(req *http.Request) (logsink.LogWriter, error) {
			s.stub.AddCall("Open")
			return &mockLogWriter{
				s.stub,
				s.written,
				recordStack,
			}, s.stub.NextErr()
		},
		s.abort,
		nil, // no rate-limiting
		metricsCollector,
		modelUUID.String(),
	))
	return srv, func() {
		finish()
		srv.Close()
	}
}

type mockLogWriter struct {
	*testing.Stub
	written  chan<- params.LogRecord
	callback func()
}

func (m *mockLogWriter) Close() error {
	m.MethodCall(m, "Close")
	if m.callback != nil {
		m.callback()
	}
	return m.NextErr()
}

func (m *mockLogWriter) WriteLog(r params.LogRecord) error {
	m.MethodCall(m, "WriteLog", r)
	m.written <- r
	return m.NextErr()
}

type slowWriteCloser struct{}

func (slowWriteCloser) Close() error {
	return nil
}

func (slowWriteCloser) WriteLog(params.LogRecord) error {
	// This makes it more likely that the goroutine will notice the
	// stop channel is closed, because logCh won't be ready for
	// sending.
	time.Sleep(testing.ShortWait)
	return nil
}

func createMockMetrics(c *tc.C, modelUUID string) (*mocks.MockMetricsCollector, func()) {
	ctrl := gomock.NewController(c)

	counter := mocks.NewMockCounter(ctrl)
	counter.EXPECT().Inc().AnyTimes()

	gauge := mocks.NewMockGauge(ctrl)
	gauge.EXPECT().Inc().AnyTimes()
	gauge.EXPECT().Dec().AnyTimes()

	metricsCollector := mocks.NewMockMetricsCollector(ctrl)
	metricsCollector.EXPECT().TotalConnections().Return(counter).AnyTimes()
	metricsCollector.EXPECT().Connections().Return(gauge).AnyTimes()
	metricsCollector.EXPECT().LogWriteCount(modelUUID, gomock.Any()).Return(counter).AnyTimes()
	metricsCollector.EXPECT().LogReadCount(modelUUID, gomock.Any()).Return(counter).AnyTimes()

	return metricsCollector, ctrl.Finish
}
