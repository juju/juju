// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"path/filepath"
	"runtime"

	jc "github.com/juju/testing/checkers"
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

func sockPath(c *gc.C) string {
	sockPath := filepath.Join(c.MkDir(), "test.listener")
	if runtime.GOOS == "windows" {
		return `\\.\pipe` + sockPath[2:]
	}
	return sockPath
}

func (s *ListenerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.socketPath = sockPath(c)
}

// Mirror the params to uniter.NewRunListener, but add cleanup to close it.
func (s *ListenerSuite) NewRunListener(c *gc.C) *uniter.RunListener {
	listener, err := uniter.NewRunListener(&mockRunner{c}, s.socketPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(listener, gc.NotNil)
	s.AddCleanup(func(*gc.C) {
		listener.Close()
	})
	return listener
}

func (s *ListenerSuite) TestNewRunListenerOnExistingSocketRemovesItAndSucceeds(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Current named pipes implementation does not support this")
	}
	s.NewRunListener(c)

	listener, err := uniter.NewRunListener(&mockRunner{}, s.socketPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(listener, gc.NotNil)
	listener.Close()
}

func (s *ListenerSuite) TestClientCall(c *gc.C) {
	s.NewRunListener(c)

	client, err := sockets.Dial(s.socketPath)
	c.Assert(err, jc.ErrorIsNil)
	defer client.Close()

	var result exec.ExecResponse
	args := uniter.RunCommandsArgs{
		Commands:        "some-command",
		RelationId:      -1,
		RemoteUnitName:  "",
		ForceRemoteUnit: false,
	}
	err = client.Call(uniter.JujuRunEndpoint, args, &result)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(string(result.Stdout), gc.Equals, "some-command stdout")
	c.Assert(string(result.Stderr), gc.Equals, "some-command stderr")
	c.Assert(result.Code, gc.Equals, 42)
}

type mockRunner struct {
	c *gc.C
}

var _ uniter.CommandRunner = (*mockRunner)(nil)

func (r *mockRunner) RunCommands(args uniter.RunCommandsArgs) (results *exec.ExecResponse, err error) {
	r.c.Log("mock runner: " + args.Commands)
	return &exec.ExecResponse{
		Code:   42,
		Stdout: []byte(args.Commands + " stdout"),
		Stderr: []byte(args.Commands + " stderr"),
	}, nil
}
