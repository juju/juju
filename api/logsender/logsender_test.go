// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	"errors"
	"io"
	"net/url"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logsender"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
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
	w, err := a.LogWriter()
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
	w, err := a.LogWriter()
	c.Assert(err, gc.ErrorMatches, "cannot connect to /logsink: foo")
	c.Assert(w, gc.Equals, nil)
}

func (s *LogSenderSuite) TestNewAPIWriteError(c *gc.C) {
	conn := &mockConnector{
		c:          c,
		writeError: errors.New("foo"),
	}
	a := logsender.NewAPI(conn)
	w, err := a.LogWriter()
	c.Assert(err, gc.IsNil)

	err = w.WriteLog(new(params.LogRecord))
	c.Assert(err, gc.ErrorMatches, "cannot send log message: foo")
	c.Assert(conn.written, gc.HasLen, 0)
}

type mockConnector struct {
	c *gc.C

	connectError error
	writeError   error
	written      []interface{}

	closeCount int
}

func (c *mockConnector) ConnectStream(path string, values url.Values) (base.Stream, error) {
	c.c.Assert(path, gc.Equals, "/logsink")
	c.c.Assert(values, jc.DeepEquals, url.Values{
		"jujuclientversion": []string{version.Current.String()},
		"version":           []string{"1"},
	})
	if c.connectError != nil {
		return nil, c.connectError
	}
	return mockStream{c}, nil
}

type mockStream struct {
	conn *mockConnector
}

func (s mockStream) WriteJSON(v interface{}) error {
	if s.conn.writeError != nil {
		return s.conn.writeError
	}
	s.conn.written = append(s.conn.written, v)
	return nil
}

func (s mockStream) ReadJSON(v interface{}) error {
	s.conn.c.Errorf("ReadJSON called unexpectedly")
	return nil
}

func (s mockStream) NextReader() (messageType int, r io.Reader, err error) {
	// NextReader is now called by the read loop thread.
	// So just wait a bit and return so it doesn't sit in a very tight loop.
	time.Sleep(time.Millisecond)
	return 0, nil, nil
}

func (s mockStream) Close() error {
	s.conn.closeCount++
	return nil
}
