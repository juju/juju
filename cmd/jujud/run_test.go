// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujuos "github.com/juju/os"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter"
)

type RunTestSuite struct {
	testing.BaseSuite

	machinelock *fakemachinelock
}

func (s *RunTestSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&cmdutil.DataDir, c.MkDir())
	s.machinelock = &fakemachinelock{}
}

var _ = gc.Suite(&RunTestSuite{})

func (*RunTestSuite) TestArgParsing(c *gc.C) {
	for i, test := range []struct {
		title           string
		args            []string
		errMatch        string
		unit            names.UnitTag
		commands        string
		avoidContext    bool
		relationId      string
		remoteUnit      string
		remoteApp       string
		forceRemoteUnit bool
		operator        bool
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
		title:    "explicit unit with no commands",
		args:     []string{"-u", "foo/2"},
		errMatch: "missing commands",
	}, {
		title:    "more than two arg",
		args:     []string{"foo/2", "bar", "baz"},
		commands: "bar baz",
		unit:     names.NewUnitTag("foo/2"),
	}, {
		title:    "command looks like unit id",
		args:     []string{"-u", "foo/2", "unit-foo-2"},
		commands: "unit-foo-2",
		unit:     names.NewUnitTag("foo/2"),
	}, {
		title:    "command looks like unit name",
		args:     []string{"-u", "foo/2", "foo/2"},
		commands: "foo/2",
		unit:     names.NewUnitTag("foo/2"),
	}, {
		title:      "unit and command assignment",
		args:       []string{"unit-name-2", "command"},
		unit:       names.NewUnitTag("name/2"),
		commands:   "command",
		relationId: "",
	}, {
		title:      "unit id converted to tag",
		args:       []string{"foo/1", "command"},
		unit:       names.NewUnitTag("foo/1"),
		commands:   "command",
		relationId: "",
	}, {
		title:      "explicit unit id converted to tag",
		args:       []string{"-u", "foo/1", "command"},
		unit:       names.NewUnitTag("foo/1"),
		commands:   "command",
		relationId: "",
	}, {
		title:      "explicit unit name converted to tag",
		args:       []string{"-u", "unit-foo-1", "command"},
		unit:       names.NewUnitTag("foo/1"),
		commands:   "command",
		relationId: "",
	}, {
		title:           "execute not in a context",
		args:            []string{"--no-context", "command"},
		commands:        "command",
		avoidContext:    true,
		relationId:      "",
		forceRemoteUnit: false,
	}, {
		title:           "relation-id",
		args:            []string{"--relation", "db:1", "unit-name-2", "command"},
		commands:        "command",
		unit:            names.NewUnitTag("name/2"),
		relationId:      "db:1",
		remoteUnit:      "",
		avoidContext:    false,
		forceRemoteUnit: false,
	}, {
		title:           "remote-unit",
		args:            []string{"--remote-unit", "name/1", "unit-name-2", "command"},
		commands:        "command",
		unit:            names.NewUnitTag("name/2"),
		avoidContext:    false,
		relationId:      "",
		remoteUnit:      "name/1",
		remoteApp:       "name",
		forceRemoteUnit: false,
	}, {
		title:           "no-remote-unit",
		args:            []string{"--force-remote-unit", "--relation", "mongodb:1", "unit-name-2", "command"},
		commands:        "command",
		unit:            names.NewUnitTag("name/2"),
		relationId:      "mongodb:1",
		forceRemoteUnit: true,
	}, {
		title:           "remote-app",
		args:            []string{"--relation", "mongodb:1", "--remote-app", "app", "name/2", "command"},
		commands:        "command",
		unit:            names.NewUnitTag("name/2"),
		relationId:      "mongodb:1",
		remoteApp:       "app",
		forceRemoteUnit: false,
	}, {
		title:      "unit id converted to tag",
		args:       []string{"--operator", "foo/1", "command"},
		unit:       names.NewUnitTag("foo/1"),
		commands:   "command",
		relationId: "",
		operator:   true,
	}, {
		title:           "execute not in a context with unit",
		args:            []string{"--no-context", "-u", "foo/1"},
		commands:        "command",
		avoidContext:    true,
		relationId:      "",
		forceRemoteUnit: false,
		errMatch:        `-no-context cannot be passed with an explicit unit-name \(-u "foo/1"\)`,
	},
	} {
		c.Logf("%d: %s", i, test.title)
		runCommand := &RunCommand{}
		err := cmdtesting.InitCommand(runCommand, test.args)
		if test.errMatch == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(runCommand.unit, gc.Equals, test.unit)
			c.Assert(runCommand.commands, gc.Equals, test.commands)
			c.Assert(runCommand.noContext, gc.Equals, test.avoidContext)
			c.Assert(runCommand.relationId, gc.Equals, test.relationId)
			c.Assert(runCommand.remoteUnitName, gc.Equals, test.remoteUnit)
			c.Assert(runCommand.remoteApplicationName, gc.Equals, test.remoteApp)
			c.Assert(runCommand.forceRemoteUnit, gc.Equals, test.forceRemoteUnit)
			c.Assert(runCommand.operator, gc.Equals, test.operator)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *RunTestSuite) runCommand() *RunCommand {
	return &RunCommand{
		MachineLock: s.machinelock,
	}
}

func (s *RunTestSuite) TestInferredUnit(c *gc.C) {
	dataDir := c.MkDir()
	runCommand := &RunCommand{dataDir: dataDir}
	err := os.MkdirAll(filepath.Join(dataDir, "agents", "unit-foo-66"), 0700)
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(runCommand, []string{"status-get"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(runCommand.unit.String(), gc.Equals, "unit-foo-66")
	c.Assert(runCommand.commands, gc.Equals, "status-get")
}

func (s *RunTestSuite) TestInsideContext(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTEXT_ID", "fake-id")
	runCommand := s.runCommand()
	err := runCommand.Init([]string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, "juju-run cannot be called from within a hook.*")
}

func (s *RunTestSuite) TestMissingAgentName(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.runCommand(), "foo/2", "bar")
	c.Assert(err, gc.ErrorMatches, `unit "foo/2" not found on this machine`)
}

func (s *RunTestSuite) TestMissingAgentTag(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.runCommand(), "unit-foo-2", "bar")
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

func (s *RunTestSuite) startRunAsync(c *gc.C, params []string) <-chan *cmd.Context {
	resultChannel := make(chan *cmd.Context)
	go func() {
		ctx, err := cmdtesting.RunCommand(c, s.runCommand(), params...)
		c.Assert(err, jc.Satisfies, cmd.IsRcPassthroughError)
		c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 0")
		resultChannel <- ctx
		close(resultChannel)
	}()
	return resultChannel
}

func (s *RunTestSuite) TestNoContext(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.runCommand(), "--no-context", "echo done")
	c.Assert(err, jc.Satisfies, cmd.IsRcPassthroughError)
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 0")
	c.Assert(strings.TrimRight(cmdtesting.Stdout(ctx), "\r\n"), gc.Equals, "done")
}

func (s *RunTestSuite) TestNoContextAsync(c *gc.C) {
	channel := s.startRunAsync(c, []string{"--no-context", "echo done"})
	ctx, err := waitForResult(channel, testing.LongWait)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimRight(cmdtesting.Stdout(ctx), "\r\n"), gc.Equals, "done")
}

func (s *RunTestSuite) TestNoContextWithLock(c *gc.C) {
	releaser, err := s.machinelock.Acquire(machinelock.Spec{})
	c.Assert(err, jc.ErrorIsNil)

	channel := s.startRunAsync(c, []string{"--no-context", "echo done"})
	_, err = waitForResult(channel, testing.ShortWait)
	c.Assert(err, gc.ErrorMatches, "timeout")

	releaser()

	ctx, err := waitForResult(channel, testing.LongWait)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimRight(cmdtesting.Stdout(ctx), "\r\n"), gc.Equals, "done")
}

func (s *RunTestSuite) TestMissingSocket(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Current implementation of named pipes loops if the socket is missing")
	}
	agentDir := filepath.Join(cmdutil.DataDir, "agents", "unit-foo-1")
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, s.runCommand(), "foo/1", "bar")
	c.Assert(err, gc.ErrorMatches, `.*dial unix .*/run.socket:.*`+utils.NoSuchFileErrRegexp)
}

func (s *RunTestSuite) TestRunning(c *gc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	ctx, err := cmdtesting.RunCommand(c, s.runCommand(), "foo/1", "bar")
	c.Check(cmd.IsRcPassthroughError(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 42")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "bar stdout")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "bar stderr")
}

func (s *RunTestSuite) TestRunningRelation(c *gc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	ctx, err := cmdtesting.RunCommand(c, s.runCommand(), "--relation", "db:1", "foo/1", "bar")
	c.Check(cmd.IsRcPassthroughError(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 42")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "bar stdout")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "bar stderr")
}

func (s *RunTestSuite) TestRunningBadRelation(c *gc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	_, err := cmdtesting.RunCommand(c, s.runCommand(), "--relation", "badrelation:W", "foo/1", "bar")
	c.Check(cmd.IsRcPassthroughError(err), jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, "invalid relation id")
}

func (s *RunTestSuite) TestRunningRemoteUnitNoRelation(c *gc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	_, err := cmdtesting.RunCommand(c, s.runCommand(), "--remote-unit", "remote/0", "foo/1", "bar")
	c.Check(cmd.IsRcPassthroughError(err), jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, "remote unit: remote/0, provided without a relation")
}

func (s *RunTestSuite) TestSkipCheckAndRemoteUnit(c *gc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	ctx, err := cmdtesting.RunCommand(c, s.runCommand(), "--force-remote-unit", "--remote-unit", "name/2", "--relation", "db:1", "foo/1", "bar")
	c.Check(cmd.IsRcPassthroughError(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "subprocess encountered error code 42")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "bar stdout")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "bar stderr")
}

func (s *RunTestSuite) TestCheckRelationIdValid(c *gc.C) {
	for i, test := range []struct {
		title  string
		input  string
		output int
		err    bool
	}{
		{
			title:  "valid, id only",
			input:  "0",
			output: 0,
			err:    false,
		}, {
			title:  "valid, relation:id",
			input:  "db:1",
			output: 1,
			err:    false,
		}, {
			title:  "not valid, just relation",
			input:  "db",
			output: -1,
			err:    true,
		}, {
			title:  "not valud, malformed relation:id",
			input:  "db:X",
			output: -1,
			err:    true,
		},
	} {
		c.Logf("%d: %s", i, test.title)
		relationId, err := checkRelationId(test.input)
		c.Assert(relationId, gc.Equals, test.output)
		if test.err {
			c.Assert(err, gc.NotNil)
		}
	}
}

func (s *RunTestSuite) runListenerForAgent(c *gc.C, agent string) {
	agentDir := filepath.Join(cmdutil.DataDir, "agents", agent)
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	socket := sockets.Socket{}
	switch jujuos.HostOS() {
	case jujuos.Windows:
		socket.Address = fmt.Sprintf(`\\.\pipe\%s-run`, agent)
	default:
		socket.Network = "unix"
		socket.Address = fmt.Sprintf("%s/run.socket", agentDir)
	}
	listener, err := uniter.NewRunListener(socket, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	listener.RegisterRunner("foo/1", &mockRunner{c})
	s.AddCleanup(func(*gc.C) {
		c.Assert(listener.Close(), jc.ErrorIsNil)
	})
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

type fakemachinelock struct {
	mu sync.Mutex
}

func (f *fakemachinelock) Acquire(spec machinelock.Spec) (func(), error) {
	f.mu.Lock()
	return func() {
		f.mu.Unlock()
	}, nil
}

func (f *fakemachinelock) Report(opts ...machinelock.ReportOption) (string, error) {
	return "", nil
}
