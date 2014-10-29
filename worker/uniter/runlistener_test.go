// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"path/filepath"
	"runtime"

	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter"
)

type ListenerSuite struct {
	testing.BaseSuite
	socketPath string
}

var _ = gc.Suite(&ListenerSuite{})

func (s *ListenerSuite) sockPath(c *gc.C) string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\testpipe`
	}
	return filepath.Join(c.MkDir(), "test.listener")
}

// Mirror the params to uniter.NewRunListener, but add cleanup to close it.
func (s *ListenerSuite) NewRunListener(c *gc.C) *uniter.RunListener {
	s.socketPath = s.sockPath(c)
	listener, err := uniter.NewRunListener(&mockRunner{c}, s.socketPath)
	c.Assert(err, gc.IsNil)
	c.Assert(listener, gc.NotNil)
	s.AddCleanup(func(*gc.C) {
		listener.Close()
	})
	return listener
}

func (s *ListenerSuite) TestNewRunListenerOnExistingSocketRemovesItAndSucceeds(c *gc.C) {
	s.NewRunListener(c)

	listener, err := uniter.NewRunListener(&mockRunner{}, s.socketPath)
	c.Assert(err, gc.IsNil)
	c.Assert(listener, gc.NotNil)
	listener.Close()
}

func (s *ListenerSuite) TestClientCall(c *gc.C) {
	s.NewRunListener(c)

	client, err := sockets.Dial(s.socketPath)
	c.Assert(err, gc.IsNil)
	defer client.Close()

	var result exec.ExecResponse
	err = client.Call(uniter.JujuRunEndpoint, "some-command", &result)
	c.Assert(err, gc.IsNil)

	c.Assert(string(result.Stdout), gc.Equals, "some-command stdout")
	c.Assert(string(result.Stderr), gc.Equals, "some-command stderr")
	c.Assert(result.Code, gc.Equals, 42)
}

type mockRunner struct {
	c *gc.C
}

var _ uniter.CommandRunner = (*mockRunner)(nil)

func (r *mockRunner) RunCommands(commands string) (results *exec.ExecResponse, err error) {
	r.c.Log("mock runner: " + commands)
	return &exec.ExecResponse{
		Code:   42,
		Stdout: []byte(commands + " stdout"),
		Stderr: []byte(commands + " stderr"),
	}, nil
}
