// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type APIOpenerSuite struct {
	// Don't need any base suites
}

var _ = gc.Suite(&APIOpenerSuite{})

func (*APIOpenerSuite) TestPassthrough(c *gc.C) {
	var controllerName, accountName, modelName string
	open := func(_ jujuclient.ClientStore, controllerNameArg, accountNameArg, modelNameArg string) (api.Connection, error) {
		controllerName = controllerNameArg
		accountName = accountNameArg
		modelName = modelNameArg
		// Lets do the bad thing and return both a connection instance
		// and an error to check they both come through.
		return &mockConnection{}, errors.New("boom")
	}
	opener := modelcmd.OpenFunc(open)
	conn, err := opener.Open(nil, "a-name", "b-name", "c-name")
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(conn, gc.NotNil)
	c.Assert(controllerName, gc.Equals, "a-name")
	c.Assert(accountName, gc.Equals, "b-name")
	c.Assert(modelName, gc.Equals, "c-name")
}

func (*APIOpenerSuite) TestTimoutSuccess(c *gc.C) {
	var controllerName, accountName, modelName string
	open := func(_ jujuclient.ClientStore, controllerNameArg, accountNameArg, modelNameArg string) (api.Connection, error) {
		controllerName = controllerNameArg
		accountName = accountNameArg
		modelName = modelNameArg
		return &mockConnection{}, nil
	}
	opener := modelcmd.NewTimeoutOpener(modelcmd.OpenFunc(open), clock.WallClock, 10*time.Second)
	conn, err := opener.Open(nil, "a-name", "b-name", "c-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.NotNil)
	c.Assert(controllerName, gc.Equals, "a-name")
	c.Assert(accountName, gc.Equals, "b-name")
	c.Assert(modelName, gc.Equals, "c-name")
}

func (*APIOpenerSuite) TestTimoutErrors(c *gc.C) {
	var controllerName, accountName, modelName string
	open := func(_ jujuclient.ClientStore, controllerNameArg, accountNameArg, modelNameArg string) (api.Connection, error) {
		controllerName = controllerNameArg
		accountName = accountNameArg
		modelName = modelNameArg
		return nil, errors.New("boom")
	}
	opener := modelcmd.NewTimeoutOpener(modelcmd.OpenFunc(open), clock.WallClock, 10*time.Second)
	conn, err := opener.Open(nil, "a-name", "b-name", "c-name")
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(conn, gc.IsNil)
	c.Assert(controllerName, gc.Equals, "a-name")
	c.Assert(accountName, gc.Equals, "b-name")
	c.Assert(modelName, gc.Equals, "c-name")
}

func (*APIOpenerSuite) TestTimoutClosesAPIOnTimeout(c *gc.C) {
	var controllerName, accountName, modelName string
	finished := make(chan struct{})
	mockConn := &mockConnection{closed: make(chan struct{})}
	open := func(_ jujuclient.ClientStore, controllerNameArg, accountNameArg, modelNameArg string) (api.Connection, error) {
		<-finished
		controllerName = controllerNameArg
		accountName = accountNameArg
		modelName = modelNameArg
		return mockConn, nil
	}
	// have the mock clock only wait a microsecond
	clock := &mockClock{wait: time.Microsecond}
	// but tell it to wait five seconds
	opener := modelcmd.NewTimeoutOpener(modelcmd.OpenFunc(open), clock, 5*time.Second)
	conn, err := opener.Open(nil, "a-name", "b-name", "c-name")
	c.Assert(errors.Cause(err), gc.Equals, modelcmd.ErrConnTimedOut)
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
	c.Assert(controllerName, gc.Equals, "a-name")
	c.Assert(accountName, gc.Equals, "b-name")
	c.Assert(modelName, gc.Equals, "c-name")
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
