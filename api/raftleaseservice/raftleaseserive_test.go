// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseservice

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type RaftLeaseServiceSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&RaftLeaseServiceSuite{})

func (s *RaftLeaseServiceSuite) TestNewAPI(c *gc.C) {
	conn := &mockConnector{
		c: c,
		readable: []params.LeaseOperationResult{
			{UUID: "aaa"},
		},
	}
	a := NewAPI(conn)
	w, err := a.OpenMessageWriter()
	c.Assert(err, gc.IsNil)

	msg := &params.LeaseOperation{
		UUID: "aaa",
	}
	err = w.Send(msg)
	c.Assert(err, gc.IsNil)

	c.Assert(conn.written, gc.HasLen, 1)
	c.Assert(conn.written[0], gc.Equals, msg)

	err = w.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn.closeCount, gc.Equals, 1)
}

func (s *RaftLeaseServiceSuite) TestNewAPIOutOfOrderResults(c *gc.C) {
	conn := &mockConnector{
		c: c,
		readable: []params.LeaseOperationResult{
			{UUID: "bbb"},
			{UUID: "aaa"},
		},
	}
	a := NewAPI(conn)
	w, err := a.OpenMessageWriter()
	c.Assert(err, gc.IsNil)

	msg := &params.LeaseOperation{
		UUID: "aaa",
	}
	err = w.Send(msg)
	c.Assert(err, gc.IsNil)

	c.Assert(conn.written, gc.HasLen, 1)
	c.Assert(conn.written[0], gc.Equals, msg)

	err = w.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn.closeCount, gc.Equals, 1)
}

func (s *RaftLeaseServiceSuite) TestNewAPIWriteLogError(c *gc.C) {
	conn := &mockConnector{
		c:            c,
		connectError: errors.New("foo"),
	}
	a := NewAPI(conn)
	w, err := a.OpenMessageWriter()
	c.Assert(err, gc.ErrorMatches, "cannot connect to /raft/lease: foo")
	c.Assert(w, gc.Equals, nil)
}

func (s *RaftLeaseServiceSuite) TestNewAPIWriteError(c *gc.C) {
	conn := &mockConnector{
		c:          c,
		writeError: errors.New("foo"),
	}
	a := NewAPI(conn)
	w, err := a.OpenMessageWriter()
	c.Assert(err, gc.IsNil)
	defer w.Close()

	err = w.Send(new(params.LeaseOperation))
	c.Assert(err, gc.ErrorMatches, "cannot send lease operation message: foo")
	c.Assert(conn.written, gc.HasLen, 0)
}

type mockConnector struct {
	c *gc.C

	connectError error
	writeError   error
	written      []interface{}
	readable     []params.LeaseOperationResult

	closeCount int
}

func (c *mockConnector) ConnectControllerStream(path string, values url.Values, headers http.Header) (base.Stream, error) {
	c.c.Assert(path, gc.Equals, "/raft/lease")
	c.c.Assert(values, gc.HasLen, 0)
	c.c.Assert(headers, gc.HasLen, 0)
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
	for _, read := range s.conn.readable {
		x := v.(*params.LeaseOperationResult)
		*x = read
	}
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
