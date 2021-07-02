// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	actionapi "github.com/juju/juju/api/action"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ExecSuite struct {
	BaseActionSuite
}

var _ = gc.Suite(&ExecSuite{})

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
			ModelType:    modelType,
			ActiveBranch: model.GenerationMaster,
		}},
	}
	store.Accounts["arthur"] = jujuclient.AccountDetails{
		User: "king",
	}
	return store
}

func (*ExecSuite) TestTargetArgParsing(c *gc.C) {
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
	}, {
		message:  "command to unit operator",
		args:     []string{"--operator", "--unit", "mysql/0", "echo hello"},
		commands: "echo hello",
		units:    []string{"mysql/0"},
		modeType: model.CAAS,
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		runCmd, execCmd := newTestExecCommand(testClock(), test.modeType)
		cmdtesting.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(execCmd.All(), gc.Equals, test.all)
			c.Check(execCmd.Machines(), gc.DeepEquals, test.machines)
			c.Check(execCmd.Applications(), gc.DeepEquals, test.applications)
			c.Check(execCmd.Units(), gc.DeepEquals, test.units)
			c.Check(execCmd.Commands(), gc.Equals, test.commands)
		}
	}
}

func (*ExecSuite) TestWaitArgParsing(c *gc.C) {
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
			c.Check(execCmd.Wait(), gc.Equals, test.wait)
		}
	}
}

func (s *ExecSuite) TestExecForMachineAndUnit(c *gc.C) {
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
	c.Check(err, jc.ErrorIsNil)

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
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expected)
}

func (s *ExecSuite) TestAllMachines(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stdout(context), gc.Equals, `
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
	c.Check(cmdtesting.Stderr(context), gc.Equals, `
Running operation 1 with 2 tasks
  - task 1 on machine-0
  - task 2 on machine-1

Waiting for task 1...
Waiting for task 2...
`[1:])
}

func (s *ExecSuite) TestTimeout(c *gc.C) {
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

	numExpectedTimers := 3
	for t := 0; t < 1; t++ {
		err2 := s.clock.WaitAdvance(2*time.Second, testing.ShortWait, numExpectedTimers)
		c.Assert(err2, jc.ErrorIsNil)
		numExpectedTimers = 1
	}
	wg.Wait()
	c.Check(err, gc.ErrorMatches, "timed out waiting for results from: machine 1, machine 2")

	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, `
Running operation 1 with 3 tasks
  - task 1 on machine-0
  - task 2 on machine-1
  - task 3 on machine-2

Waiting for task 1...
Waiting for task 2...
`[1:])
}

func (s *ExecSuite) TestCAASCantTargetMachine(c *gc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	runCmd, _ := newTestExecCommand(testClock(), model.CAAS)
	_, err := cmdtesting.RunCommand(c, runCmd,
		"--machine", "0", "echo hello",
	)

	expErr := "unable to target machines with a k8s controller"
	c.Assert(err, gc.ErrorMatches, expErr)
}

func (s *ExecSuite) TestIAASCantTargetOperator(c *gc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
	_, err := cmdtesting.RunCommand(c, runCmd,
		"--unit", "unit/0", "--operator", "echo hello",
	)

	expErr := "only k8s models support the --operator flag"
	c.Assert(err, gc.ErrorMatches, expErr)
}

func (s *ExecSuite) TestCAASExecOnOperator(c *gc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
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

	runCmd, _ := newTestExecCommand(testClock(), model.CAAS)
	context, err := cmdtesting.RunCommand(c, runCmd,
		"--format=yaml", "--unit=mysql/0", "--operator", "hostname", "--utc",
	)
	c.Assert(err, jc.ErrorIsNil)

	parallel := true
	group := ""
	c.Assert(fakeClient.execParams, jc.DeepEquals, &actionapi.RunParams{
		Commands:        "hostname",
		Timeout:         300 * time.Second,
		Units:           []string{"mysql/0"},
		WorkloadContext: false,
		Parallel:        &parallel,
		ExecutionGroup:  &group,
	})

	expectedOutput := `
mysql/0:
  id: "1"
  results:
    stdout: bumblebee
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
  unit: mysql/0
`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expectedOutput)
}

func (s *ExecSuite) TestCAASExecOnWorkload(c *gc.C) {
	fakeClient := &fakeAPIClient{}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	fakeClient.actionResults = []actionapi.ActionResult{{
		Action: &actionapi.Action{
			ID:       validActionId,
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

	runCmd, _ := newTestExecCommand(testClock(), model.CAAS)
	context, err := cmdtesting.RunCommand(c, runCmd,
		"--format=yaml", "--unit=mysql/0", "hostname", "--utc", "--execution-group", "group",
	)
	c.Assert(err, jc.ErrorIsNil)

	parallel := true
	group := "group"
	c.Check(fakeClient.execParams, jc.DeepEquals, &actionapi.RunParams{
		Commands:        "hostname",
		Timeout:         300 * time.Second,
		Units:           []string{"mysql/0"},
		WorkloadContext: true,
		Parallel:        &parallel,
		ExecutionGroup:  &group,
	})

	expectedOutput := `
mysql/0:
  id: "1"
  results:
    stdout: bumblebee
  status: completed
  timing:
    completed: 2015-02-14 08:17:00 +0000 UTC
    enqueued: 2015-02-14 08:13:00 +0000 UTC
    started: 2015-02-14 08:15:00 +0000 UTC
  unit: mysql/0
`[1:]
	c.Assert(cmdtesting.Stdout(context), gc.Equals, expectedOutput)
}

func testClock() *testclock.Clock {
	return testclock.NewClock(time.Now())
}

func (s *ExecSuite) TestBlockAllMachines(c *gc.C) {
	fakeClient := &fakeAPIClient{block: true}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
	_, err := cmdtesting.RunCommand(c, runCmd, "--format=json", "--all", "hostname")
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *ExecSuite) TestBlockExecForMachineAndUnit(c *gc.C) {
	fakeClient := &fakeAPIClient{block: true}
	restore := s.patchAPIClient(fakeClient)
	defer restore()

	runCmd, _ := newTestExecCommand(testClock(), model.IAAS)
	_, err := cmdtesting.RunCommand(c, runCmd,
		"--format=json", "--machine=0", "--unit=unit/0", "hostname",
	)
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *ExecSuite) TestSingleResponse(c *gc.C) {
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

	stdErr := `
Running operation 1 with 1 task
  - task 1 on machine-0

Waiting for task 1...
`[1:]

	for i, test := range []struct {
		message string
		format  string
		stdout  string
		stderr  string
		err     string
	}{{
		message: "smart (default)",
		stdout:  "\n",
		stderr:  stdErr,
		err:     `task failed with exit code: 42`,
	}, {
		message: "yaml output",
		format:  "yaml",
		stdout:  yamlFormatted,
		stderr:  stdErr,
		err:     `task failed with exit code: 42`,
	}, {
		message: "json output",
		format:  "json",
		stdout:  jsonFormatted,
		stderr:  stdErr,
		err:     `task failed with exit code: 42`,
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
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(cmdtesting.Stdout(context), gc.Equals, test.stdout)
		c.Check(cmdtesting.Stderr(context), gc.Equals, test.stderr)
	}
}
