// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package run

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/exec"

	"github.com/juju/juju/agent/config"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter"
	"github.com/juju/juju/juju/sockets"
)

type RunTestSuite struct {
	testing.BaseSuite

	machinelock *fakemachinelock
}

func (s *RunTestSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&config.DataDir, c.MkDir())
	s.machinelock = &fakemachinelock{}
}
func TestRunTestSuite(t *stdtesting.T) {
	tc.Run(t, &RunTestSuite{})
}

func (*RunTestSuite) TestArgParsing(c *tc.C) {
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
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(runCommand.unit, tc.Equals, test.unit)
			c.Assert(runCommand.commands, tc.Equals, test.commands)
			c.Assert(runCommand.noContext, tc.Equals, test.avoidContext)
			c.Assert(runCommand.relationId, tc.Equals, test.relationId)
			c.Assert(runCommand.remoteUnitName, tc.Equals, test.remoteUnit)
			c.Assert(runCommand.remoteApplicationName, tc.Equals, test.remoteApp)
			c.Assert(runCommand.forceRemoteUnit, tc.Equals, test.forceRemoteUnit)
		} else {
			c.Assert(err, tc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *RunTestSuite) runCommand() *RunCommand {
	return &RunCommand{
		MachineLock: s.machinelock,
	}
}

func (s *RunTestSuite) TestInferredUnit(c *tc.C) {
	dataDir := c.MkDir()
	runCommand := &RunCommand{dataDir: dataDir}
	err := os.MkdirAll(filepath.Join(dataDir, "agents", "unit-foo-66"), 0700)
	c.Assert(err, tc.ErrorIsNil)
	err = cmdtesting.InitCommand(runCommand, []string{"status-get"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(runCommand.unit.String(), tc.Equals, "unit-foo-66")
	c.Assert(runCommand.commands, tc.Equals, "status-get")
}

func (s *RunTestSuite) TestInsideContext(c *tc.C) {
	s.PatchEnvironment("JUJU_CONTEXT_ID", "fake-id")
	runCommand := s.runCommand()
	err := runCommand.Init([]string{"foo", "bar"})
	c.Assert(err, tc.ErrorMatches, "juju-exec cannot be called from within a hook.*")
}

func (s *RunTestSuite) TestMissingAgentName(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.runCommand(), "foo/2", "bar")
	c.Assert(err, tc.ErrorMatches, `unit "foo/2" not found on this machine`)
}

func (s *RunTestSuite) TestMissingAgentTag(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.runCommand(), "unit-foo-2", "bar")
	c.Assert(err, tc.ErrorMatches, `unit "foo/2" not found on this machine`)
}

func waitForResult(running <-chan *cmd.Context, timeout time.Duration) (*cmd.Context, error) {
	select {
	case result := <-running:
		return result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}

func (s *RunTestSuite) startRunAsync(c *tc.C, params []string) <-chan *cmd.Context {
	resultChannel := make(chan *cmd.Context)
	go func() {
		ctx, err := cmdtesting.RunCommand(c, s.runCommand(), params...)
		c.Assert(err, tc.Satisfies, utils.IsRcPassthroughError)
		c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 0")
		resultChannel <- ctx
		close(resultChannel)
	}()
	return resultChannel
}

func (s *RunTestSuite) TestNoContext(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.runCommand(), "--no-context", "echo done")
	c.Assert(err, tc.Satisfies, utils.IsRcPassthroughError)
	c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 0")
	c.Assert(strings.TrimRight(cmdtesting.Stdout(ctx), "\r\n"), tc.Equals, "done")
}

func (s *RunTestSuite) TestNoContextAsync(c *tc.C) {
	channel := s.startRunAsync(c, []string{"--no-context", "echo done"})
	ctx, err := waitForResult(channel, testing.LongWait)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(strings.TrimRight(cmdtesting.Stdout(ctx), "\r\n"), tc.Equals, "done")
}

func (s *RunTestSuite) TestNoContextWithLock(c *tc.C) {
	releaser, err := s.machinelock.Acquire(machinelock.Spec{})
	c.Assert(err, tc.ErrorIsNil)

	channel := s.startRunAsync(c, []string{"--no-context", "echo done"})
	_, err = waitForResult(channel, testing.ShortWait)
	c.Assert(err, tc.ErrorMatches, "timeout")

	releaser()

	ctx, err := waitForResult(channel, testing.LongWait)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(strings.TrimRight(cmdtesting.Stdout(ctx), "\r\n"), tc.Equals, "done")
}

func (s *RunTestSuite) TestMissingSocket(c *tc.C) {
	agentDir := filepath.Join(config.DataDir, "agents", "unit-foo-1")
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, tc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, s.runCommand(), "foo/1", "bar")
	c.Assert(err, tc.ErrorMatches, `.*/run.socket:.*`+utils.NoSuchFileErrRegexp)
}

func (s *RunTestSuite) TestRunning(c *tc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	ctx, err := cmdtesting.RunCommand(c, s.runCommand(), "foo/1", "bar")
	c.Check(utils.IsRcPassthroughError(err), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 42")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "bar stdout")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "bar stderr")
}

func (s *RunTestSuite) TestRunningRelation(c *tc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	ctx, err := cmdtesting.RunCommand(c, s.runCommand(), "--relation", "db:1", "foo/1", "bar")
	c.Check(utils.IsRcPassthroughError(err), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 42")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "bar stdout")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "bar stderr")
}

func (s *RunTestSuite) TestRunningBadRelation(c *tc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	_, err := cmdtesting.RunCommand(c, s.runCommand(), "--relation", "badrelation:W", "foo/1", "bar")
	c.Check(utils.IsRcPassthroughError(err), tc.IsFalse)
	c.Assert(err, tc.ErrorMatches, "invalid relation id")
}

func (s *RunTestSuite) TestRunningRemoteUnitNoRelation(c *tc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	_, err := cmdtesting.RunCommand(c, s.runCommand(), "--remote-unit", "remote/0", "foo/1", "bar")
	c.Check(utils.IsRcPassthroughError(err), tc.IsFalse)
	c.Assert(err, tc.ErrorMatches, "remote unit: remote/0, provided without a relation")
}

func (s *RunTestSuite) TestSkipCheckAndRemoteUnit(c *tc.C) {
	loggo.GetLogger("worker.uniter").SetLogLevel(loggo.TRACE)
	s.runListenerForAgent(c, "unit-foo-1")

	ctx, err := cmdtesting.RunCommand(c, s.runCommand(), "--force-remote-unit", "--remote-unit", "name/2", "--relation", "db:1", "foo/1", "bar")
	c.Check(utils.IsRcPassthroughError(err), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, "subprocess encountered error code 42")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "bar stdout")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "bar stderr")
}

func (s *RunTestSuite) TestCheckRelationIdValid(c *tc.C) {
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
		c.Assert(relationId, tc.Equals, test.output)
		if test.err {
			c.Assert(err, tc.NotNil)
		}
	}
}

func (s *RunTestSuite) runListenerForAgent(c *tc.C, agent string) {
	agentDir := filepath.Join(config.DataDir, "agents", agent)
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, tc.ErrorIsNil)
	socket := sockets.Socket{}
	socket.Network = "unix"
	socket.Address = path.Join(agentDir, "run.socket")
	listener, err := uniter.NewRunListener(socket, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	listener.RegisterRunner("foo/1", &mockRunner{c})
	s.AddCleanup(func(c *tc.C) {
		c.Assert(listener.Close(), tc.ErrorIsNil)
	})
}

type mockRunner struct {
	c *tc.C
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
