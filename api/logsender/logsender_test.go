// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	"errors"
	"net/url"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logsender"
	"github.com/juju/juju/apiserver/params"
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
	c.c.Assert(values, gc.HasLen, 0)
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

func (s mockStream) Read([]byte) (int, error) {
	s.conn.c.Errorf("Read called unexpectedly")
	return 0, nil
}

func (s mockStream) Write([]byte) (int, error) {
	s.conn.c.Errorf("Write called unexpectedly")
	return 0, nil
}

func (s mockStream) Close() error {
	s.conn.closeCount++
	return nil
}
