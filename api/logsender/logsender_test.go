// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	"context"
	"errors"
	"io"
	"net/url"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logsender"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type LogSenderSuite struct {
	coretesting.BaseSuite
}

func TestLogSenderSuite(t *stdtesting.T) {
	tc.Run(t, &LogSenderSuite{})
}

func (s *LogSenderSuite) TestNewAPI(c *tc.C) {
	conn := &mockConnector{
		c: c,
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter(c.Context())
	c.Assert(err, tc.IsNil)

	msg := new(params.LogRecord)
	err = w.WriteLog(msg)
	c.Assert(err, tc.IsNil)

	c.Assert(conn.written, tc.HasLen, 1)
	c.Assert(conn.written[0], tc.Equals, msg)

	err = w.Close()
	c.Assert(err, tc.IsNil)
	c.Assert(conn.closeCount, tc.Equals, 1)
}

func (s *LogSenderSuite) TestNewAPIWriteLogError(c *tc.C) {
	conn := &mockConnector{
		c:            c,
		connectError: errors.New("foo"),
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter(c.Context())
	c.Assert(err, tc.ErrorMatches, "cannot connect to /logsink: foo")
	c.Assert(w, tc.Equals, nil)
}

func (s *LogSenderSuite) TestNewAPIWriteError(c *tc.C) {
	conn := &mockConnector{
		c:          c,
		writeError: errors.New("foo"),
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter(c.Context())
	c.Assert(err, tc.IsNil)

	err = w.WriteLog(new(params.LogRecord))
	c.Assert(err, tc.ErrorMatches, "sending log message: foo")
	c.Assert(conn.written, tc.HasLen, 0)
}

func (s *LogSenderSuite) TestNewAPIReadError(c *tc.C) {
	conn := &mockConnector{
		c:          c,
		closed:     make(chan bool),
		readError:  errors.New("read foo"),
		writeError: errors.New("closed yo"),
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter(c.Context())
	c.Assert(err, tc.IsNil)
	select {
	case <-conn.closed:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timeout waiting for connection to close")
	}

	err = w.WriteLog(new(params.LogRecord))
	c.Assert(err, tc.ErrorMatches, "sending log message: read foo: closed yo")
	c.Assert(conn.written, tc.HasLen, 0)
}

type mockConnector struct {
	c *tc.C

	connectError error
	writeError   error
	readError    error
	written      []interface{}
	readDone     chan struct{}
	closeCount   int
	closed       chan bool
}

func (c *mockConnector) ConnectStream(_ context.Context, path string, values url.Values) (base.Stream, error) {
	c.c.Assert(path, tc.Equals, "/logsink")
	c.c.Assert(values, tc.DeepEquals, url.Values{
		"version": []string{"1"},
	})

	if c.connectError != nil {
		return nil, c.connectError
	}

	c.readDone = make(chan struct{}, 1)
	return mockStream{conn: c, closed: c.closed}, nil
}

type mockStream struct {
	conn   *mockConnector
	closed chan bool
}

func (s mockStream) NextReader() (messageType int, r io.Reader, err error) {
	defer func() {
		select {
		case s.conn.readDone <- struct{}{}:
		default:
		}
	}()

	// NextReader is now called by the read loop thread.
	// Wait a bit before returning, so it doesn't sit in a very tight loop.
	time.Sleep(time.Millisecond)

	if s.conn.readError != nil {
		return 0, nil, s.conn.readError
	}
	return 0, nil, nil
}

func (s mockStream) WriteJSON(v interface{}) error {
	// Wait for a NextReader call in case the test
	// orchestration is for an error there.
	select {
	case <-s.conn.readDone:
	case <-time.After(coretesting.LongWait):
		s.conn.c.Errorf("timed out waiting for read")
	}

	if s.conn.writeError != nil {
		return s.conn.writeError
	}
	s.conn.written = append(s.conn.written, v)
	return nil
}

func (s mockStream) Close() error {
	s.conn.closeCount++
	if s.closed != nil {
		close(s.closed)
	}
	return nil
}

func (s mockStream) ReadJSON(v interface{}) error {
	s.conn.c.Errorf("ReadJSON called unexpectedly")
	return nil
}
