// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	"context"
	"errors"
	"io"
	"net/url"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logsender"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type LogSenderSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&LogSenderSuite{})

func (s *LogSenderSuite) TestNewAPI(c *gc.C) {
	conn := &mockConnector{
		c: c,
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter(context.Background())
	c.Assert(err, gc.IsNil)

	msg := new(params.LogRecord)
	err = w.WriteLog(msg)
	c.Assert(err, gc.IsNil)

	c.Assert(conn.written, gc.HasLen, 1)
	c.Assert(conn.written[0], gc.Equals, msg)

	err = w.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn.closeCount, gc.Equals, 1)
}

func (s *LogSenderSuite) TestNewAPIWriteLogError(c *gc.C) {
	conn := &mockConnector{
		c:            c,
		connectError: errors.New("foo"),
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter(context.Background())
	c.Assert(err, gc.ErrorMatches, "cannot connect to /logsink: foo")
	c.Assert(w, gc.Equals, nil)
}

func (s *LogSenderSuite) TestNewAPIWriteError(c *gc.C) {
	conn := &mockConnector{
		c:          c,
		writeError: errors.New("foo"),
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter(context.Background())
	c.Assert(err, gc.IsNil)

	err = w.WriteLog(new(params.LogRecord))
	c.Assert(err, gc.ErrorMatches, "sending log message: foo")
	c.Assert(conn.written, gc.HasLen, 0)
}

func (s *LogSenderSuite) TestNewAPIReadError(c *gc.C) {
	conn := &mockConnector{
		c:          c,
		closed:     make(chan bool),
		readError:  errors.New("read foo"),
		writeError: errors.New("closed yo"),
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter(context.Background())
	c.Assert(err, gc.IsNil)
	select {
	case <-conn.closed:
	case <-time.After(testing.LongWait):
		c.Fatal("timeout waiting for connection to close")
	}

	err = w.WriteLog(new(params.LogRecord))
	c.Assert(err, gc.ErrorMatches, "sending log message: read foo: closed yo")
	c.Assert(conn.written, gc.HasLen, 0)
}

type mockConnector struct {
	c *gc.C

	connectError error
	writeError   error
	readError    error
	written      []interface{}
	readDone     chan struct{}
	closeCount   int
	closed       chan bool
}

func (c *mockConnector) ConnectStream(_ context.Context, path string, values url.Values) (base.Stream, error) {
	c.c.Assert(path, gc.Equals, "/logsink")
	c.c.Assert(values, jc.DeepEquals, url.Values{
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
