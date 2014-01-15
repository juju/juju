// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/exec"
	"launchpad.net/juju-core/worker/uniter"
)

type RunTestSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&RunTestSuite{})

func (*RunTestSuite) TestWrongArgs(c *gc.C) {
	for i, test := range []struct {
		title    string
		args     []string
		errMatch string
		unit     string
		commands string
	}{{
		title:    "no args",
		errMatch: "missing unit-name",
	}, {
		title:    "one arg",
		args:     []string{"foo"},
		errMatch: "missing commands",
	}, {
		title:    "more than two arg",
		args:     []string{"foo", "bar", "baz"},
		errMatch: `unrecognized args: \["baz"\]`,
	}, {
		title:    "unit and command assignment",
		args:     []string{"unit-name", "command"},
		unit:     "unit-name",
		commands: "command",
	}, {
		title:    "unit id converted to tag",
		args:     []string{"foo/1", "command"},
		unit:     "unit-foo-1",
		commands: "command",
	},
	} {
		c.Log(fmt.Sprintf("\n%d: %s", i, test.title))
		runCommand := &RunCommand{}
		err := runCommand.Init(test.args)
		if test.errMatch == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(runCommand.unit, gc.Equals, test.unit)
			c.Assert(runCommand.commands, gc.Equals, test.commands)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *RunTestSuite) TestInsideContext(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTEXT_ID", "fake-id")
	runCommand := &RunCommand{}
	err := runCommand.Init([]string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, "juju-run cannot be called from within a hook.*")
}

func (s *RunTestSuite) TestMissingAgent(c *gc.C) {
	agentDir := c.MkDir()
	s.PatchValue(&AgentDir, agentDir)

	_, err := testing.RunCommand(c, &RunCommand{}, []string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, `unit "foo" not found on this machine`)
}

func (s *RunTestSuite) TestMissingSocket(c *gc.C) {
	s.PatchValue(&AgentDir, c.MkDir())
	testAgentDir := filepath.Join(AgentDir, "foo")
	err := os.Mkdir(testAgentDir, 0755)
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &RunCommand{}, []string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, `dial unix .*/run.socket: no such file or directory`)
}

func (s *RunTestSuite) TestRunning(c *gc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "foo")

	ctx, err := testing.RunCommand(c, &RunCommand{}, []string{"foo", "bar"})
	c.Check(cmd.IsRcPassthroughError(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "rc: 42")
	c.Assert(testing.Stdout(ctx), gc.Equals, "bar stdout")
	c.Assert(testing.Stderr(ctx), gc.Equals, "bar stderr")
}

func (s *RunTestSuite) runListenerForAgent(c *gc.C, agent string) {
	s.PatchValue(&AgentDir, c.MkDir())

	testAgentDir := filepath.Join(AgentDir, agent)
	err := os.Mkdir(testAgentDir, 0755)
	c.Assert(err, gc.IsNil)

	socketPath := filepath.Join(testAgentDir, uniter.RunListenerFile)
	listener, err := uniter.NewRunListener(&mockRunner{c}, "unix", socketPath)
	c.Assert(err, gc.IsNil)
	c.Assert(listener, gc.NotNil)
	s.AddCleanup(func(*gc.C) {
		listener.Close()
	})
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
