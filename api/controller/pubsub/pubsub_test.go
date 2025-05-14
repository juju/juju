// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub_test

import (
	"context"
	"errors"
	"io"
	"net/url"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	apipubsub "github.com/juju/juju/api/controller/pubsub"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type PubSubSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&PubSubSuite{})

func (s *PubSubSuite) TestNewAPI(c *tc.C) {
	conn := &mockConnector{
		c: c,
	}
	a := apipubsub.NewAPI(conn)
	w, err := a.OpenMessageWriter(c.Context())
	c.Assert(err, tc.IsNil)

	msg := new(params.PubSubMessage)
	err = w.ForwardMessage(msg)
	c.Assert(err, tc.IsNil)

	c.Assert(conn.written, tc.HasLen, 1)
	c.Assert(conn.written[0], tc.Equals, msg)

	err = w.Close()
	c.Assert(err, tc.IsNil)
	c.Assert(conn.closeCount, tc.Equals, 1)
}

func (s *PubSubSuite) TestNewAPIWriteLogError(c *tc.C) {
	conn := &mockConnector{
		c:            c,
		connectError: errors.New("foo"),
	}
	a := apipubsub.NewAPI(conn)
	w, err := a.OpenMessageWriter(c.Context())
	c.Assert(err, tc.ErrorMatches, "cannot connect to /pubsub: foo")
	c.Assert(w, tc.Equals, nil)
}

func (s *PubSubSuite) TestNewAPIWriteError(c *tc.C) {
	conn := &mockConnector{
		c:          c,
		writeError: errors.New("foo"),
	}
	a := apipubsub.NewAPI(conn)
	w, err := a.OpenMessageWriter(c.Context())
	c.Assert(err, tc.IsNil)
	defer w.Close()

	err = w.ForwardMessage(new(params.PubSubMessage))
	c.Assert(err, tc.ErrorMatches, "cannot send pubsub message: foo")
	c.Assert(conn.written, tc.HasLen, 0)
}

type mockConnector struct {
	c *tc.C

	connectError error
	writeError   error
	written      []interface{}

	closeCount int
}

func (c *mockConnector) ConnectStream(_ context.Context, path string, values url.Values) (base.Stream, error) {
	c.c.Assert(path, tc.Equals, "/pubsub")
	c.c.Assert(values, tc.HasLen, 0)
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
