// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"net/rpc"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/worker/uniter"
)

type ListenerSuite struct {
	testbase.LoggingSuite
	socketPath string
}

var _ = gc.Suite(&ListenerSuite{})

// Mirror the params to uniter.NewRunListener, but add cleanup to close it.
func (s *ListenerSuite) NewRunListener(c *gc.C) *uniter.RunListener {
	s.socketPath = filepath.Join(c.MkDir(), "test.listener")
	listener, err := uniter.NewRunListener(&mockRunner{c}, "unix", s.socketPath)
	c.Assert(err, gc.IsNil)
	c.Assert(listener, gc.NotNil)
	s.AddCleanup(func(*gc.C) {
		listener.Close()
	})
	return listener
}

func (s *ListenerSuite) TestNewRunListenerSecondFails(c *gc.C) {
	s.NewRunListener(c)

	listener, err := uniter.NewRunListener(&mockRunner{}, "unix", s.socketPath)

	c.Assert(listener, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".* address already in use")
}

func (s *ListenerSuite) TestClientCall(c *gc.C) {
	s.NewRunListener(c)

	client, err := rpc.Dial("unix", s.socketPath)
	c.Assert(err, gc.IsNil)
	defer client.Close()

	var result cmd.RemoteResponse
	err = client.Call("Runner.RunCommands", "some-command", &result)
	c.Assert(err, gc.IsNil)

	c.Assert(string(result.Stdout), gc.Equals, "some-command stdout")
	c.Assert(string(result.Stderr), gc.Equals, "some-command stderr")
	c.Assert(result.Code, gc.Equals, 42)
}

type mockRunner struct {
	c *gc.C
}

var _ uniter.CommandRunner = (*mockRunner)(nil)

func (r *mockRunner) RunCommands(commands string) (results *cmd.RemoteResponse, err error) {
	r.c.Log("mock runner: " + commands)
	return &cmd.RemoteResponse{
		Code:   42,
		Stdout: []byte(commands + " stdout"),
		Stderr: []byte(commands + " stderr"),
	}, nil
}
