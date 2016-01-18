// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
)

type APIOpenerSuite struct {
	// Don't need any base suites
}

var _ = gc.Suite(&APIOpenerSuite{})

func (*APIOpenerSuite) TestPassthrough(c *gc.C) {
	var name string
	open := func(connectionName string) (api.Connection, error) {
		name = connectionName
		// Lets do the bad thing and return both a connection instance
		// and an error to check they both come through.
		return &mockConnection{}, errors.New("boom")
	}
	opener := envcmd.NewPassthroughOpener(open)
	conn, err := opener.Open("a-name")
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(conn, gc.NotNil)
	c.Assert(name, gc.Equals, "a-name")
}

func (*APIOpenerSuite) TestTimoutSuccess(c *gc.C) {
	var name string
	open := func(connectionName string) (api.Connection, error) {
		name = connectionName
		return &mockConnection{}, nil
	}
	opener := envcmd.NewTimeoutOpener(open, clock.WallClock, 10*time.Second)
	conn, err := opener.Open("a-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.NotNil)
	c.Assert(name, gc.Equals, "a-name")
}

func (*APIOpenerSuite) TestTimoutErrors(c *gc.C) {
	var name string
	open := func(connectionName string) (api.Connection, error) {
		name = connectionName
		return nil, errors.New("boom")
	}
	opener := envcmd.NewTimeoutOpener(open, clock.WallClock, 10*time.Second)
	conn, err := opener.Open("a-name")
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(conn, gc.IsNil)
	c.Assert(name, gc.Equals, "a-name")
}

func (*APIOpenerSuite) TestTimoutClosesAPIOnTimeout(c *gc.C) {
	var name string
	finished := make(chan struct{})
	mockConn := &mockConnection{closed: make(chan struct{})}
	open := func(connectionName string) (api.Connection, error) {
		<-finished
		name = connectionName
		return mockConn, nil
	}
	// have the mock clock only wait a microsecond
	clock := &mockClock{wait: time.Microsecond}
	// but tell it to wait five seconds
	opener := envcmd.NewTimeoutOpener(open, clock, 5*time.Second)
	conn, err := opener.Open("a-name")
	c.Assert(errors.Cause(err), gc.Equals, envcmd.ErrConnTimedOut)
	c.Assert(conn, gc.IsNil)
	// check it was told to wait for 5 seconds
	c.Assert(clock.duration, gc.Equals, 5*time.Second)
	// tell the open func to continue now we have timed out
	close(finished)
	// wait until the connection has been closed
	select {
	case <-mockConn.closed:
		// continue
	case <-time.After(5 * time.Second):
		c.Error("API connection was not closed.")
	}
	c.Assert(name, gc.Equals, "a-name")
}

// mockConnection will panic if anything but close is called.
type mockConnection struct {
	api.Connection

	closed chan struct{}
}

func (m *mockConnection) Close() error {
	close(m.closed)
	return nil
}

// mockClock will panic if anything but After is called
type mockClock struct {
	clock.Clock

	wait     time.Duration
	duration time.Duration
}

func (m *mockClock) After(duration time.Duration) <-chan time.Time {
	m.duration = duration
	return time.After(m.wait)
}
