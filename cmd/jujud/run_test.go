// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/fslock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter"
)

type RunTestSuite struct {
	testing.BaseSuite
}

func (s *RunTestSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&DataDir, c.MkDir())
}

var _ = gc.Suite(&RunTestSuite{})

func (*RunTestSuite) TestArgParsing(c *gc.C) {
	for i, test := range []struct {
		title        string
		args         []string
		errMatch     string
		unit         names.UnitTag
		commands     string
		avoidContext bool
	}{{
		title:    "no args",
		errMatch: "missing unit-name",
	}, {
		title:    "one arg",
		args:     []string{"foo"},
		errMatch: `"foo" is not a valid tag`,
	}, {
		title:    "one arg",
		args:     []string{"foo/2"},
		errMatch: "missing commands",
	}, {
		title:    "more than two arg",
		args:     []string{"foo/2", "bar", "baz"},
		errMatch: `unrecognized args: \["baz"\]`,
	}, {
		title:    "unit and command assignment",
		args:     []string{"unit-name-2", "command"},
		unit:     names.NewUnitTag("name/2"),
		commands: "command",
	}, {
		title:    "unit id converted to tag",
		args:     []string{"foo/1", "command"},
		unit:     names.NewUnitTag("foo/1"),
		commands: "command",
	}, {
		title:        "execute not in a context",
		args:         []string{"--no-context", "command"},
		commands:     "command",
		avoidContext: true,
	},
	} {
		c.Logf("\n%d: %s", i, test.title)
		runCommand := &RunCommand{}
		err := testing.InitCommand(runCommand, test.args)
		if test.errMatch == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(runCommand.unit, gc.Equals, test.unit)
			c.Assert(runCommand.commands, gc.Equals, test.commands)
			c.Assert(runCommand.noContext, gc.Equals, test.avoidContext)
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

func (s *RunTestSuite) TestMissingAgentName(c *gc.C) {
	_, err := testing.RunCommand(c, &RunCommand{}, "foo/2", "bar")
	c.Assert(err, gc.ErrorMatches, `unit "foo/2" not found on this machine`)
}

func (s *RunTestSuite) TestMissingAgentTag(c *gc.C) {
	_, err := testing.RunCommand(c, &RunCommand{}, "unit-foo-2", "bar")
	c.Assert(err, gc.ErrorMatches, `unit "foo/2" not found on this machine`)
}

func waitForResult(running <-chan *cmd.Context, timeout time.Duration) (*cmd.Context, error) {
	select {
	case result := <-running:
		return result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}

func startRunAsync(c *gc.C, params []string) <-chan *cmd.Context {
	resultChannel := make(chan *cmd.Context)
	go func() {
		ctx, err := testing.RunCommand(c, &RunCommand{}, params...)
		c.Assert(err, jc.Satisfies, cmd.IsRcPassthroughError)
		c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 0")
		resultChannel <- ctx
		close(resultChannel)
	}()
	return resultChannel
}

func (s *RunTestSuite) TestNoContext(c *gc.C) {
	ctx, err := testing.RunCommand(c, &RunCommand{}, "--no-context", "echo done")
	c.Assert(err, jc.Satisfies, cmd.IsRcPassthroughError)
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 0")
	c.Assert(testing.Stdout(ctx), gc.Equals, "done\n")
}

func (s *RunTestSuite) TestNoContextAsync(c *gc.C) {
	channel := startRunAsync(c, []string{"--no-context", "echo done"})
	ctx, err := waitForResult(channel, testing.LongWait)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "done\n")
}

func (s *RunTestSuite) TestNoContextWithLock(c *gc.C) {
	s.PatchValue(&fslock.LockWaitDelay, 10*time.Millisecond)

	lock, err := hookExecutionLock(dataDir)
	c.Assert(err, gc.IsNil)
	lock.Lock("juju-run test")
	defer lock.Unlock() // in case of failure

	channel := startRunAsync(c, []string{"--no-context", "echo done"})
	ctx, err := waitForResult(channel, testing.ShortWait)
	c.Assert(err, gc.ErrorMatches, "timeout")

	lock.Unlock()

	ctx, err = waitForResult(channel, testing.LongWait)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "done\n")
}

func (s *RunTestSuite) TestMissingSocket(c *gc.C) {
	agentDir := filepath.Join(DataDir, "agents", "unit-foo-1")
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &RunCommand{}, "foo/1", "bar")
	c.Assert(err, gc.ErrorMatches, `dial unix .*/run.socket: no such file or directory`)
}

func (s *RunTestSuite) TestRunning(c *gc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	ctx, err := testing.RunCommand(c, &RunCommand{}, "foo/1", "bar")
	c.Check(cmd.IsRcPassthroughError(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 42")
	c.Assert(testing.Stdout(ctx), gc.Equals, "bar stdout")
	c.Assert(testing.Stderr(ctx), gc.Equals, "bar stderr")
}

func (s *RunTestSuite) runListenerForAgent(c *gc.C, agent string) {
	agentDir := filepath.Join(DataDir, "agents", agent)
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, gc.IsNil)
	var socketPath string
	switch version.Current.OS {
	case version.Windows:
		socketPath = fmt.Sprintf(`\\.\pipe\%s-run`, agent)
	default:
		socketPath = fmt.Sprintf("%s/run.socket", agentDir)
	}
	listener, err := uniter.NewRunListener(&mockRunner{c}, socketPath)
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
