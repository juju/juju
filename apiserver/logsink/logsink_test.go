// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/websocket/websockettest"
	coretesting "github.com/juju/juju/testing"
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

	srv   *httptest.Server
	abort chan struct{}

	mu      sync.Mutex
	opened  int
	closed  int
	stub    testing.Stub
	written chan params.LogRecord

	logs loggo.TestWriter
}

var _ = gc.Suite(&logsinkSuite{})

func (s *logsinkSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.abort = make(chan struct{})
	s.written = make(chan params.LogRecord, 1)
	s.stub.ResetCalls()
	s.srv = httptest.NewServer(logsink.NewHTTPHandler(
		func(req *http.Request) (logsink.LogWriteCloser, error) {
			s.stub.AddCall("Open")
			return &mockLogWriteCloser{
				&s.stub,
				s.written,
			}, s.stub.NextErr()
		},
		s.abort,
	))
	s.AddCleanup(func(*gc.C) { s.srv.Close() })
}

func (s *logsinkSuite) dialWebsocket(c *gc.C) *websocket.Conn {
	u, err := url.Parse(s.srv.URL)
	c.Assert(err, jc.ErrorIsNil)
	u.Scheme = "ws"
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { conn.Close() })
	return conn
}

func (s *logsinkSuite) TestSuccess(c *gc.C) {
	conn := s.dialWebsocket(c)
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
	c.Assert(err, jc.ErrorIsNil)

	select {
	case written, ok := <-s.written:
		c.Assert(ok, jc.IsTrue)
		c.Assert(written, jc.DeepEquals, record)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for log record to be written")
	}
	select {
	case <-s.written:
		c.Fatal("unexpected log record")
	case <-time.After(coretesting.ShortWait):
	}
	s.stub.CheckCallNames(c, "Open", "WriteLog")

	err = conn.Close()
	c.Assert(err, jc.ErrorIsNil)
	for a := longAttempt.Start(); a.Next(); {
		if len(s.stub.Calls()) == 3 {
			break
		}
	}
	s.stub.CheckCallNames(c, "Open", "WriteLog", "Close")
}

func (s *logsinkSuite) TestLogMessages(c *gc.C) {
	var logs loggo.TestWriter
	writer := loggo.NewMinimumLevelWriter(&logs, loggo.INFO)
	c.Assert(loggo.RegisterWriter("logsink-tests", writer), jc.ErrorIsNil)

	// Open, then close connection.
	conn := s.dialWebsocket(c)
	websockettest.AssertJSONInitialErrorNil(c, conn)
	err := conn.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that no error is logged when the connection is closed normally.
	for a := shortAttempt.Start(); a.Next(); {
		for _, log := range logs.Log() {
			c.Assert(log.Level, jc.LessThan, loggo.ERROR, gc.Commentf("log: %#v", log))
		}
	}
}

func (s *logsinkSuite) TestLogOpenFails(c *gc.C) {
	s.stub.SetErrors(errors.New("rats"))
	conn := s.dialWebsocket(c)
	websockettest.AssertJSONError(c, conn, "rats")
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *logsinkSuite) TestLogWriteFails(c *gc.C) {
	s.stub.SetErrors(nil, errors.New("cannae write"))
	conn := s.dialWebsocket(c)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	err := conn.WriteJSON(&params.LogRecord{})
	c.Assert(err, jc.ErrorIsNil)

	websockettest.AssertJSONError(c, conn, "cannae write")
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *logsinkSuite) TestReceiveErrorBreaksConn(c *gc.C) {
	conn := s.dialWebsocket(c)
	websockettest.AssertJSONInitialErrorNil(c, conn)

	// The logsink handler expects JSON messages. Send some
	// junk to verify that the server closes the connection.
	err := conn.WriteMessage(websocket.TextMessage, []byte("junk!"))
	c.Assert(err, jc.ErrorIsNil)

	websockettest.AssertWebsocketClosed(c, conn)
}

type mockLogWriteCloser struct {
	*testing.Stub
	written chan<- params.LogRecord
}

func (m *mockLogWriteCloser) Close() error {
	m.MethodCall(m, "Close")
	return m.NextErr()
}

func (m *mockLogWriteCloser) WriteLog(r params.LogRecord) error {
	m.MethodCall(m, "WriteLog", r)
	m.written <- r
	return m.NextErr()
}
