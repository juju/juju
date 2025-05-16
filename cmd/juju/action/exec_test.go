// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"bytes"
	"fmt"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type ExecSuite struct {
	BaseActionSuite
}

func TestExecSuite(t *stdtesting.T) { tc.Run(t, &ExecSuite{}) }
func newTestExecCommand(clock clock.Clock, modelType model.ModelType) (cmd.Command, *action.ExecCommand) {
	return action.NewExecCommandForTest(minimalStore(modelType), clock, nil)
}

func minimalStore(modelType model.ModelType) *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "arthur"
	store.Controllers["arthur"] = jujuclient.ControllerDetails{}
	store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: modelType,
		}},
	}
	store.Accounts["arthur"] = jujuclient.AccountDetails{
		User: "king",
	}
	return store
}

func (*ExecSuite) TestTargetArgParsing(c *tc.C) {
	for i, test := range []struct {
		message      string
		args         []string
		all          bool
		machines     []string
		units        []string
		applications []string
		commands     string
		errMatch     string
		modeType     model.ModelType
	}{{
		message:  "no args",
		errMatch: "no commands specified",
		modeType: model.IAAS,
	}, {
		message:  "no target",
		args:     []string{"sudo reboot"},
		errMatch: "You must specify a target, either through --all, --machine, --application or --unit",
		modeType: model.IAAS,
	}, {
		message:  "command to all machines",
		args:     []string{"--all", "sudo reboot"},
		all:      true,
		commands: "sudo reboot",
		modeType: model.IAAS,
	}, {
		message:  "multiple args",
		args:     []string{"--all", "echo", "la lia"},
		all:      true,
		commands: `echo "la lia"`,
		modeType: model.IAAS,
	}, {
		message:  "all and defined machines",
		args:     []string{"--all", "--machine=1,2", "sudo reboot"},
		errMatch: `You cannot specify --all and individual machines`,
		modeType: model.IAAS,
	}, {
		message:  "command to machines 1, 2, and 1/kvm/0",
		args:     []string{"--machine=1,2,1/kvm/0", "sudo reboot"},
		commands: "sudo reboot",
		machines: []string{"1", "2", "1/kvm/0"},
		modeType: model.IAAS,
	}, {
		message: "bad machine names",
		args:    []string{"--machine=foo,machine-2", "sudo reboot"},
		errMatch: "" +
			"The following exec targets are not valid:\n" +
			"  \"foo\" is not a valid machine id\n" +
			"  \"machine-2\" is not a valid machine id",
		modeType: model.IAAS,
	}, {
		message:  "all and defined applications",
		args:     []string{"--all", "--application=wordpress,mysql", "sudo reboot"},
		errMatch: `You cannot specify --all and individual applications`,
		modeType: model.IAAS,
	}, {
		message:      "command to applications wordpress and mysql",
		args:         []string{"--application=wordpress,mysql", "sudo reboot"},
		commands:     "sudo reboot",
		applications: []string{"wordpress", "mysql"},
		modeType:     model.IAAS,
	}, {
		message:      "command to application mysql",
		args:         []string{"--app", "mysql", "uname -a"},
		commands:     "uname -a",
		applications: []string{"mysql"},
		modeType:     model.IAAS,
	}, {
		message: "bad application names",
		args:    []string{"--application", "foo,2,foo/0", "sudo reboot"},
		errMatch: "" +
			"The following exec targets are not valid:\n" +
			"  \"2\" is not a valid application name\n" +
			"  \"foo/0\" is not a valid application name",
		modeType: model.IAAS,
	}, {
		message:      "command to application mysql",
		args:         []string{"--app", "mysql", "sudo reboot"},
		commands:     "sudo reboot",
		applications: []string{"mysql"},
		modeType:     model.IAAS,
	}, {
		message:      "command to application wordpress",
		args:         []string{"-a", "wordpress", "sudo reboot"},
		commands:     "sudo reboot",
		applications: []string{"wordpress"},
		modeType:     model.IAAS,
	}, {
		message:  "all and defined units",
		args:     []string{"--all", "--unit=wordpress/0,mysql/1", "sudo reboot"},
		errMatch: `You cannot specify --all and individual units`,
		modeType: model.IAAS,
	}, {
		message:  "command to valid unit",
		args:     []string{"-u", "mysql/0", "sudo reboot"},
		commands: "sudo reboot",
		units:    []string{"mysql/0"},
		modeType: model.IAAS,
	}, {
		message:  "command to valid units",
		args:     []string{"--unit=wordpress/0,wordpress/1,mysql/0", "sudo reboot"},
		commands: "sudo reboot",
		units:    []string{"wordpress/0", "wordpress/1", "mysql/0"},
		modeType: model.IAAS,
	}, {
		message: "bad unit names",
		args:    []string{"--unit", "foo,2,foo/0,foo/$leader", "sudo reboot"},
		errMatch: "" +
			"The following exec targets are not valid:\n" +
			"  \"foo\" is not a valid unit name\n" +
			"  \"2\" is not a valid unit name\n" +
			"  \"foo/\\$leader\" is not a valid unit name",
		modeType: model.IAAS,
	}, {
		message:      "command to mixed valid targets",
		args:         []string{"--machine=0", "--unit=wordpress/0,wordpress/1,consul/leader", "--application=mysql", "sudo reboot"},
		commands:     "sudo reboot",
		machines:     []string{"0"},
		applications: []string{"mysql"},
		units:        []string{"wordpress/0", "wordpress/1", "consul/leader"},
		modeType:     model.IAAS,
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		runCmd, execCmd := newTestExecCommand(testClock(), test.modeType)
		cmdtesting.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(execCmd.All(), tc.Equals, test.all)
			c.Check(execCmd.Machines(), tc.DeepEquals, test.machines)
			c.Check(execCmd.Applications(), tc.DeepEquals, test.applications)
			c.Check(execCmd.Units(), tc.DeepEquals, test.units)
			c.Check(execCmd.Commands(), tc.Equals, test.commands)
		}
	}
}

func (*ExecSuite) TestWaitArgParsing(c *tc.C) {
	for i, test := range []struct {
		message  string
		args     []string
		errMatch string
		wait     time.Duration
		modeType model.ModelType
	}{{
		message:  "default time",
		args:     []string{"--all", "sudo reboot"},
		wait:     5 * time.Minute,
		modeType: model.IAAS,
	}, {
		message:  "invalid time",
		args:     []string{"--wait=foo", "--all", "sudo reboot"},
		errMatch: `invalid value "foo" for option --wait: time: invalid duration "?foo"?`,
		modeType: model.IAAS,
	}, {
		message:  "two hours",
		args:     []string{"--wait=2h", "--all", "sudo reboot"},
		wait:     2 * time.Hour,
		modeType: model.IAAS,
	}, {
		message:  "3 minutes 30 seconds",
		args:     []string{"--wait=3m30s", "--all", "sudo reboot"},
		wait:     (3 * time.Minute) + (30 * time.Second),
		modeType: model.IAAS,
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		runCmd, execCmd := newTestExecCommand(testClock(), test.modeType)
		cmdtesting.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(execCmd.Wait(), tc.Equals, test.wait)
		}
	}
}

func (s *ExecSuite) TestExecForMachineAndUnit(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
			Receiver: "machine-0",
		},
		Output: map[string]interface{}{
			"stdout": "megatron",
		},
		Status:    "completed",
		Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
	}, {
		Action: &actionapi.Action{
			ID:       validActionId2,
			Receiver: "unit-mysql-0",
		},
		Output: map[string]interface{}{
			"stdout": "bumblebee",
		},
		Status:    "completed",
		Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
	}}

	runCmd, _ := newTestExecCommand(testClock(), model.IAAS)

	context, err := cmdtesting.RunCommand(c, runCmd,
		"--format=yaml", "--machine=0", "--unit=mysql/0", "hostname", "--utc",
	)
	c.Check(err, tc.ErrorIsNil)

	expected := `
"0":
  id: "1"
  machine: "0"
  results:
    stdout: megatron
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
mysql/0:
  id: "2"
  results:
    stdout: bumblebee
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
  unit: mysql/0
`[1:]
	c.Assert(cmdtesting.Stdout(context), tc.Equals, expected)
}

func (s *ExecSuite) TestAllMachines(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
			Receiver: "machine-0",
		},
		Output: map[string]interface{}{
			"stdout": "megatron",
		},
		Status:    "completed",
		Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
	}, {
		Action: &actionapi.Action{
			ID:       validActionId2,
			Receiver: "machine-1",
		},
		Status:    "completed",
		Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
	}}
	fakeClient.machines = set.NewStrings("0", "1")

	runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
	context, err := cmdtesting.RunCommand(c, runCmd,
		"--format=yaml", "--all", "hostname", "--utc")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(cmdtesting.Stdout(context), tc.Equals, `
"0":
  id: "1"
  machine: "0"
  results:
    stdout: megatron
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
"1":
  id: "2"
  machine: "1"
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
`[1:])
	c.Check(cmdtesting.Stderr(context), tc.Equals, "")
}

func (s *ExecSuite) TestAllMachinesWithError(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
			Receiver: "machine-0",
		},
		Output: map[string]interface{}{
			"stdout":      "megatron",
			"return-code": "2",
		},
		Status:    "completed",
		Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
	}, {
		Action: &actionapi.Action{
			ID:       validActionId2,
			Receiver: "machine-1",
		},
		Output: map[string]interface{}{
			"return-code": "1",
		},
		Status:    "completed",
		Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
	}}
	fakeClient.machines = set.NewStrings("0", "1")

	runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
	context, err := cmdtesting.RunCommand(c, runCmd,
		"--format=yaml", "--all", "hostname", "--utc")
	c.Assert(err, tc.ErrorMatches, `the following tasks failed:
 - id "1" with return code 2
 - id "2" with return code 1

use 'juju show-task' to inspect the failures
`)

	c.Check(cmdtesting.Stdout(context), tc.Equals, `
"0":
  id: "1"
  machine: "0"
  results:
    return-code: 2
    stdout: megatron
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
"1":
  id: "2"
  machine: "1"
  results:
    return-code: 1
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
`[1:])
	c.Check(cmdtesting.Stderr(context), tc.Equals, "")
}

func (s *ExecSuite) TestTimeout(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
			Receiver: "machine-0",
		},
		Output: map[string]interface{}{
			"stdout": "megatron",
		},
		Status:    "completed",
		Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
	}, {
		Action: &actionapi.Action{
			ID:       validActionId2,
			Receiver: "machine-1",
		},
		Status:   "pending",
		Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
	}, {
		Action: &actionapi.Action{
			ID:       validActionId3,
			Receiver: "machine-2",
		},
		Status:   "running",
		Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:  time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
	}}
	fakeClient.machines = set.NewStrings("0", "1", "2")

	s.clock = testClock()
	runCmd, _ := newTestExecCommand(s.clock, model.IAAS)

	var (
		wg  sync.WaitGroup
		ctx *cmd.Context
		err error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, err = cmdtesting.RunCommand(c, runCmd,
			"--format=yaml", "--all", "hostname", "--wait", "2s",
		)
	}()

	wg.Wait()
	c.Check(err, tc.ErrorMatches, "timed out waiting for results from: machine 1, machine 2")

	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *ExecSuite) TestVerbosity(c *tc.C) {
	tests := []struct {
		about          string
		args           []string
		verbose, quiet bool
		output, error  string
	}{{
		about:  "normal output",
		args:   []string{"--machine=0", "marco"},
		output: "polo\n",
	}, {
		about:   "verbose",
		args:    []string{"--machine=0", "marco"},
		verbose: true,
		output: `
Running operation 1 with 1 task
  - task 1 on machine-0

Waiting for task 1...
polo
`[1:],
	}, {
		about:  "quiet",
		args:   []string{"--machine=0", "marco"},
		quiet:  true,
		output: "polo\n",
	}, {
		about: "background",
		args:  []string{"--machine=0", "marco", "--background"},
		output: `
Scheduled operation 1 with task 1
Check operation status with 'juju show-operation 1'
Check task status with 'juju show-task 1'
`[1:],
	}, {
		about:   "background verbose",
		args:    []string{"--machine=0", "marco", "--background"},
		verbose: true,
		output: `
Scheduled operation 1 with task 1
Check operation status with 'juju show-operation 1'
Check task status with 'juju show-task 1'
`[1:],
	}, {
		about:  "background quiet",
		args:   []string{"--machine=0", "marco", "--background"},
		quiet:  true,
		output: "",
	}, {
		about:  "command failure",
		args:   []string{"--machine=1", "marco"},
		output: "I failed you\n",
		error: `(?m)the following task failed:
 - id "2" with return code 1

use 'juju show-task' to inspect the failure
`,
	}, {
		about:   "command failure verbose",
		args:    []string{"--machine=1", "marco"},
		verbose: true,
		output: `
Running operation 1 with 1 task
  - task 2 on machine-1

Waiting for task 2...
I failed you
`[1:],
		error: `(?m)the following task failed:
 - id "2" with return code 1

use 'juju show-task' to inspect the failure
`,
	}, {
		about:  "command failure quiet",
		args:   []string{"--machine=1", "marco"},
		quiet:  true,
		output: "I failed you\n",
		error: `(?m)the following task failed:
 - id "2" with return code 1

use 'juju show-task' to inspect the failure
`,
	}, {
		about:  "failed to schedule",
		args:   []string{"--machine=2", "marco"},
		output: "Operation 1 failed to schedule any tasks:\n",
	}}

	// Set up fake API client
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		// machine 0 is control
		Action: &actionapi.Action{
			ID:       validActionId,
			Receiver: "machine-0",
		},
		Output: map[string]interface{}{
			"stdout": "polo",
		},
	}, {
		// machine 1: command fails returning exit code 1
		Action: &actionapi.Action{
			ID:       validActionId2,
			Receiver: "machine-1",
		},
		Output: map[string]interface{}{
			"stdout":      "I failed you",
			"return-code": "1",
		},
	}}
	fakeClient.machines = set.NewStrings("0", "1")

	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)

		// Set up context
		output := bytes.Buffer{}
		ctx := &cmd.Context{
			Context: c.Context(),
			Stdout:  &output,
			Stderr:  &output,
		}
		log := cmd.Log{
			Verbose: t.verbose,
			Quiet:   t.quiet,
		}
		log.Start(ctx) // sets the verbose/quiet options in `ctx`

		// Run command
		runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
		err := cmdtesting.InitCommand(runCmd, t.args)
		c.Assert(err, tc.ErrorIsNil)

		err = runCmd.Run(ctx)
		if t.error == "" {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Assert(err, tc.NotNil)
			c.Check(err, tc.ErrorMatches, t.error)
		}

		c.Check(output.String(), tc.Equals, t.output)
	}
}

func (s *ExecSuite) TestCAASCantTargetMachine(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	runCmd, _ := newTestExecCommand(testClock(), model.CAAS)
	_, err := cmdtesting.RunCommand(c, runCmd,
		"--machine", "0", "echo hello",
	)

	expErr := "unable to target machines with a k8s controller"
	c.Assert(err, tc.ErrorMatches, expErr)
}

func testClock() testclock.AdvanceableClock {
	return testclock.NewDilatedWallClock(100 * time.Millisecond)
}

func (s *ExecSuite) TestBlockAllMachines(c *tc.C) {
	fakeClient := &fakeAPIClient{block: true}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
	_, err := cmdtesting.RunCommand(c, runCmd, "--format=json", "--all", "hostname")
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *ExecSuite) TestBlockExecForMachineAndUnit(c *tc.C) {
	fakeClient := &fakeAPIClient{block: true}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
	_, err := cmdtesting.RunCommand(c, runCmd,
		"--format=json", "--machine=0", "--unit=unit/0", "hostname",
	)
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *ExecSuite) TestSingleResponse(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
			Receiver: "machine-0",
		},
		Output: map[string]interface{}{
			"return-code": 42,
		},
		Status:    "completed",
		Enqueued:  time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		Started:   time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
		Completed: time.Date(2015, time.February, 14, 8, 17, 0, 0, time.UTC),
	}}
	fakeClient.machines = set.NewStrings("0")

	jsonFormatted := `
{"0":{"id":"1","machine":"0","results":{"return-code":42},"status":"completed","timing":{"completed":"2015-02-14 08:17:00 +0000 UTC","enqueued":"2015-02-14 08:13:00 +0000 UTC","started":"2015-02-14 08:15:00 +0000 UTC"}}}
`[1:]

	yamlFormatted := `
"0":
  id: "1"
  machine: "0"
  results:
    return-code: 42
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
`[1:]

	errStr := `
the following task failed:
 - id "1" with return code 42

use 'juju show-task' to inspect the failure
`[1:]

	for i, test := range []struct {
		message string
		format  string
		stdout  string
		err     string
	}{{
		message: "smart (default)",
		stdout:  "",
		err:     errStr,
	}, {
		message: "yaml output",
		format:  "yaml",
		stdout:  yamlFormatted,
		err:     errStr,
	}, {
		message: "json output",
		format:  "json",
		stdout:  jsonFormatted,
		err:     errStr,
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		args := []string{}
		if test.format != "" {
			args = append(args, "--format", test.format)
		}
		args = append(args, "--all", "ignored", "--utc")
		runCmd, _ := newTestExecCommand(testClock(), model.IAAS)

		context, err := cmdtesting.RunCommand(c, runCmd, args...)
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Check(err, tc.ErrorIsNil)
		}
		c.Check(cmdtesting.Stdout(context), tc.Equals, test.stdout)
		c.Check(cmdtesting.Stderr(context), tc.Equals, "")
	}
}

func (s *ExecSuite) TestMultipleUnitsPlainOutput(c *tc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
			Receiver: "unit-foo-7",
		},
		Output: map[string]interface{}{
			"stdout": "result7",
		},
	}, {
		Action: &actionapi.Action{
			ID:       validActionId2,
			Receiver: "unit-foo-34",
		},
		Output: map[string]interface{}{
			"stderr": "result34\n",
		},
	}, {
		Action: &actionapi.Action{
			ID:       validActionId3,
			Receiver: "unit-foo-112",
		},
		Output: map[string]interface{}{
			"stdout": "result112",
		},
	}}

	// Various outputs depending on what units are specified
	outputs := map[string]string{
		"--unit=foo/34": `
result34
`[1:],
		"--unit=foo/7,foo/112,foo/34": `
foo/7:
result7

foo/34:
result34

foo/112:
result112

`[1:],
	}

	for unitFlag, stdout := range outputs {
		runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
		context, err := cmdtesting.RunCommand(c, runCmd,
			"--format=plain", unitFlag, "do-stuff")
		c.Assert(err, tc.ErrorIsNil)

		c.Check(cmdtesting.Stdout(context), tc.Equals, stdout)
		c.Check(cmdtesting.Stderr(context), tc.Equals, "")
	}

}
