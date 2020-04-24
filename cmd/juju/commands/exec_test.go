// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ExecSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ExecSuite{})

func (s *ExecSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func newTestExecCommand(clock clock.Clock, modelType model.ModelType) cmd.Command {
	return newExecCommand(minimalStore(modelType), clock.After, false)
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
		cmd := &execCommand{}
		cmd.SetClientStore(minimalStore(test.modeType))
		runCmd := modelcmd.Wrap(cmd)
		cmdtesting.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(cmd.all, gc.Equals, test.all)
			c.Check(cmd.machines, gc.DeepEquals, test.machines)
			c.Check(cmd.applications, gc.DeepEquals, test.applications)
			c.Check(cmd.units, gc.DeepEquals, test.units)
			c.Check(cmd.commands, gc.Equals, test.commands)
		}
	}
}

func (*ExecSuite) TestTimeoutArgParsing(c *gc.C) {
	for i, test := range []struct {
		message  string
		args     []string
		errMatch string
		timeout  time.Duration
		modeType model.ModelType
	}{{
		message:  "default time",
		args:     []string{"--all", "sudo reboot"},
		timeout:  5 * time.Minute,
		modeType: model.IAAS,
	}, {
		message:  "invalid time",
		args:     []string{"--timeout=foo", "--all", "sudo reboot"},
		errMatch: `invalid value "foo" for option --timeout: time: invalid duration foo`,
		modeType: model.IAAS,
	}, {
		message:  "two hours",
		args:     []string{"--timeout=2h", "--all", "sudo reboot"},
		timeout:  2 * time.Hour,
		modeType: model.IAAS,
	}, {
		message:  "3 minutes 30 seconds",
		args:     []string{"--timeout=3m30s", "--all", "sudo reboot"},
		timeout:  (3 * time.Minute) + (30 * time.Second),
		modeType: model.IAAS,
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		cmd := &execCommand{}
		cmd.SetClientStore(minimalStore(test.modeType))
		runCmd := modelcmd.Wrap(cmd)
		cmdtesting.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(cmd.timeout, gc.Equals, test.timeout)
		}
	}
}

func (s *ExecSuite) TestConvertRunResults(c *gc.C) {
	for i, test := range []struct {
		message  string
		results  params.ActionResult
		query    actionQuery
		expected map[string]interface{}
	}{{
		message: "in case of error we print receiver and failed action id",
		results: makeActionResult(mockResponse{
			error: &params.Error{
				Message: "whoops",
			},
		}, ""),
		query: makeActionQuery(validUUID, "MachineId", names.NewMachineTag("1")),
		expected: map[string]interface{}{
			"Error":     "whoops",
			"MachineId": "1",
			"Action":    validUUID,
		},
	}, {
		message: "different action tag from query tag",
		results: makeActionResult(mockResponse{machineTag: "not-a-tag"}, "invalid"),
		query:   makeActionQuery(validUUID, "MachineId", names.NewMachineTag("1")),
		expected: map[string]interface{}{
			"Error":     `expected action tag "action-` + validUUID + `", got "invalid"`,
			"MachineId": "1",
			"Action":    validUUID,
		},
	}, {
		message: "different response tag from query tag",
		results: makeActionResult(mockResponse{machineTag: "not-a-tag"}, "action-"+validUUID),
		query:   makeActionQuery(validUUID, "MachineId", names.NewMachineTag("1")),
		expected: map[string]interface{}{
			"Error":     `expected action receiver "machine-1", got "not-a-tag"`,
			"MachineId": "1",
			"Action":    validUUID,
		},
	}, {
		message: "minimum is machine id",
		results: makeActionResult(mockResponse{machineTag: "machine-1"}, "action-"+validUUID),
		query:   makeActionQuery(validUUID, "MachineId", names.NewMachineTag("1")),
		expected: map[string]interface{}{
			"MachineId": "1",
		},
	}, {
		message: "other fields are copied if there",
		results: makeActionResult(mockResponse{
			unitTag: "unit-unit-0",
			stdout:  "stdout",
			stderr:  "stderr",
			message: "msg",
			code:    "42",
		}, "action-"+validUUID),
		query: makeActionQuery(validUUID, "UnitId", names.NewUnitTag("unit/0")),
		expected: map[string]interface{}{
			"unit":        "unit/0",
			"stdout":      "stdout",
			"stderr":      "stderr",
			"message":     "msg",
			"return-code": 42,
		},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		result := ConvertActionResults(test.results, test.query, false)
		c.Check(result, jc.DeepEquals, test.expected)
	}
}

func (s *ExecSuite) TestExecForMachineAndUnit(c *gc.C) {
	mock := s.setupMockAPI()
	machineResponse := mockResponse{
		stdout:     "megatron\n",
		machineTag: "machine-0",
	}
	unitResponse := mockResponse{
		stdout:  "bumblebee",
		unitTag: "unit-unit-0",
	}
	mock.setResponse("0", machineResponse)
	mock.setResponse("unit/0", unitResponse)

	machineResult := mock.execResponses["0"]
	unitResult := mock.execResponses["unit/0"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["0"]:      machineResult,
		mock.receiverIdMap["unit/0"]: unitResult,
	}

	machineQuery := makeActionQuery(mock.receiverIdMap["0"], "MachineId", names.NewMachineTag("0"))
	unitQuery := makeActionQuery(mock.receiverIdMap["unit/0"], "UnitId", names.NewUnitTag("unit/0"))
	unformatted := []interface{}{
		ConvertActionResults(machineResult, machineQuery, false),
		ConvertActionResults(unitResult, unitQuery, false),
	}

	buff := &bytes.Buffer{}
	err := cmd.FormatJson(buff, unformatted)
	c.Assert(err, jc.ErrorIsNil)

	context, err := cmdtesting.RunCommand(c, newTestExecCommand(&mockClock{}, model.IAAS),
		"--format=json", "--machine=0", "--unit=unit/0", "hostname",
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stdout(context), gc.Equals, buff.String())
}

func (s *ExecSuite) TestBlockExecForMachineAndUnit(c *gc.C) {
	mock := s.setupMockAPI()
	// Block operation
	mock.block = true
	_, err := cmdtesting.RunCommand(c, newTestExecCommand(&mockClock{}, model.IAAS),
		"--format=json", "--machine=0", "--unit=unit/0", "hostname",
	)
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *ExecSuite) TestAllMachines(c *gc.C) {
	mock := s.setupMockAPI()
	mock.setMachinesAlive("0", "1", "2")
	response0 := mockResponse{
		stdout:     "megatron\n",
		machineTag: "machine-0",
	}
	response1 := mockResponse{
		message:    "command timed out",
		machineTag: "machine-1",
	}
	response2 := mockResponse{
		message:    "command timed out",
		machineTag: "machine-2",
	}
	mock.setResponse("0", response0)
	mock.setResponse("1", response1)
	mock.setResponse("2", response2)

	machine0Result := mock.execResponses["0"]
	machine1Result := mock.execResponses["1"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["0"]: machine0Result,
		mock.receiverIdMap["1"]: machine1Result,
	}

	machine0Query := makeActionQuery(mock.receiverIdMap["0"], "MachineId", names.NewMachineTag("0"))
	machine1Query := makeActionQuery(mock.receiverIdMap["1"], "MachineId", names.NewMachineTag("1"))
	unformatted := []interface{}{
		ConvertActionResults(machine0Result, machine0Query, false),
		ConvertActionResults(machine1Result, machine1Query, false),
		map[string]interface{}{
			"Action":    mock.receiverIdMap["2"],
			"MachineId": "2",
			"Error":     "action not found",
		},
	}

	buff := &bytes.Buffer{}
	err := cmd.FormatJson(buff, unformatted)
	c.Assert(err, jc.ErrorIsNil)

	context, err := cmdtesting.RunCommand(c, newTestExecCommand(&mockClock{}, model.IAAS), "--format=json", "--all", "hostname")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stdout(context), gc.Equals, buff.String())
	c.Check(cmdtesting.Stderr(context), gc.Equals, "")
}

func (s *ExecSuite) TestTimeout(c *gc.C) {
	mock := s.setupMockAPI()
	mock.setMachinesAlive("0", "1", "2")
	response0 := mockResponse{
		stdout:     "megatron\n",
		machineTag: "machine-0",
	}
	response1 := mockResponse{
		machineTag: "machine-1",
		status:     params.ActionPending,
	}
	response2 := mockResponse{
		machineTag: "machine-2",
		status:     params.ActionRunning,
	}
	mock.setResponse("0", response0)
	mock.setResponse("1", response1)
	mock.setResponse("2", response2)

	machine0Result := mock.execResponses["0"]
	machine1Result := mock.execResponses["1"]
	machine2Result := mock.execResponses["1"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["0"]: machine0Result,
		mock.receiverIdMap["1"]: machine1Result,
		mock.receiverIdMap["2"]: machine2Result,
	}

	machine0Query := makeActionQuery(mock.receiverIdMap["0"], "MachineId", names.NewMachineTag("0"))

	var buf bytes.Buffer
	err := cmd.FormatJson(&buf, []interface{}{
		ConvertActionResults(machine0Result, machine0Query, false),
	})
	c.Assert(err, jc.ErrorIsNil)

	var clock mockClock
	context, err := cmdtesting.RunCommand(
		c, newTestExecCommand(&clock, model.IAAS),
		"--format=json", "--all", "hostname", "--timeout", "99s",
	)
	c.Assert(err, gc.ErrorMatches, "timed out waiting for results from: machine 1, machine 2")

	c.Check(cmdtesting.Stdout(context), gc.Equals, buf.String())
	c.Check(cmdtesting.Stderr(context), gc.Equals, "")
	clock.CheckCalls(c, []gitjujutesting.StubCall{
		{"After", []interface{}{99 * time.Second}},
		{"After", []interface{}{1 * time.Second}},
		{"After", []interface{}{1 * time.Second}},
	})
}

func (s *ExecSuite) TestUnitLeaderSyntaxWithUnsupportedAPIVersion(c *gc.C) {
	var (
		clock mockClock
		mock  = s.setupMockAPI()
	)

	mock.bestAPIVersion = 2
	_, err := cmdtesting.RunCommand(
		c, newTestExecCommand(&clock, model.IAAS),
		"--unit", "foo/leader", "hostname",
	)

	expErr := fmt.Sprintf("unable to determine leader for application %q"+
		"\nleader determination is unsupported by this API"+
		"\neither upgrade your controller, or explicitly specify a unit", "foo")
	c.Assert(err, gc.ErrorMatches, expErr)
}

func (s *ExecSuite) TestCAASCantExecWithUnsupportedAPIVersion(c *gc.C) {
	var (
		clock mockClock
		mock  = s.setupMockAPI()
	)

	mock.bestAPIVersion = 3
	_, err := cmdtesting.RunCommand(
		c, newTestExecCommand(&clock, model.CAAS),
		"--unit", "unit/0", "echo hello",
	)

	expErr := "k8s controller does not support juju exec\n" +
		"consider upgrading your controller"
	c.Assert(err, gc.ErrorMatches, expErr)
}

func (s *ExecSuite) TestCAASCantTargetMachine(c *gc.C) {
	s.setupMockAPI()
	var clock mockClock

	_, err := cmdtesting.RunCommand(
		c, newTestExecCommand(&clock, model.CAAS),
		"--machine", "0", "echo hello",
	)

	expErr := "unable to target machines with a k8s controller"
	c.Assert(err, gc.ErrorMatches, expErr)
}

func (s *ExecSuite) TestIAASCantTargetOperator(c *gc.C) {
	s.setupMockAPI()
	var clock mockClock

	_, err := cmdtesting.RunCommand(
		c, newTestExecCommand(&clock, model.IAAS),
		"--unit", "unit/0", "--operator", "echo hello",
	)

	expErr := "only k8s models support the --operator flag"
	c.Assert(err, gc.ErrorMatches, expErr)
}

func (s *ExecSuite) TestCAASExecOnOperator(c *gc.C) {
	mock := s.setupMockAPI()
	unitResponse := mockResponse{
		stdout:  "bumblebee",
		unitTag: "unit-unit-0",
	}
	mock.setResponse("unit/0", unitResponse)

	unitResult := mock.execResponses["unit/0"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["unit/0"]: unitResult,
	}

	unitQuery := makeActionQuery(mock.receiverIdMap["unit/0"], "UnitId", names.NewUnitTag("unit/0"))
	unformatted := []interface{}{
		ConvertActionResults(unitResult, unitQuery, false),
	}

	buff := &bytes.Buffer{}
	err := cmd.FormatJson(buff, unformatted)
	c.Assert(err, jc.ErrorIsNil)

	context, err := cmdtesting.RunCommand(c, newTestExecCommand(&mockClock{}, model.CAAS),
		"--format=json", "--unit=unit/0", "--operator", "hostname",
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mock.execParams, jc.DeepEquals, &params.RunParams{
		Commands:        "hostname",
		Timeout:         300 * time.Second,
		Units:           []string{"unit/0"},
		WorkloadContext: false,
	})

	c.Check(cmdtesting.Stdout(context), gc.Equals, buff.String())
}

func (s *ExecSuite) TestCAASExecOnWorkload(c *gc.C) {
	mock := s.setupMockAPI()
	unitResponse := mockResponse{
		stdout:  "bumblebee",
		unitTag: "unit-unit-0",
	}
	mock.setResponse("unit/0", unitResponse)

	unitResult := mock.execResponses["unit/0"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["unit/0"]: unitResult,
	}

	unitQuery := makeActionQuery(mock.receiverIdMap["unit/0"], "UnitId", names.NewUnitTag("unit/0"))
	unformatted := []interface{}{
		ConvertActionResults(unitResult, unitQuery, false),
	}

	buff := &bytes.Buffer{}
	err := cmd.FormatJson(buff, unformatted)
	c.Assert(err, jc.ErrorIsNil)

	context, err := cmdtesting.RunCommand(c, newTestExecCommand(&mockClock{}, model.CAAS),
		"--format=json", "--unit=unit/0", "hostname",
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mock.execParams, jc.DeepEquals, &params.RunParams{
		Commands:        "hostname",
		Timeout:         300 * time.Second,
		Units:           []string{"unit/0"},
		WorkloadContext: true,
	})

	c.Check(cmdtesting.Stdout(context), gc.Equals, buff.String())
}

type mockClock struct {
	gitjujutesting.Stub
	clock.Clock
	timeoutCh chan time.Time
}

func (c *mockClock) After(d time.Duration) <-chan time.Time {
	c.MethodCall(c, "After", d)
	ch := make(chan time.Time)
	if d == time.Second {
		// This is a sleepy sleep call, while we're waiting
		// for actions to be run. We simulate sleeping a
		// couple of times, and then close the timeout
		// channel.
		if len(c.Calls()) >= 3 {
			close(c.timeoutCh)
		} else {
			close(ch)
		}
	} else {
		// This is the initial time.After call for the timeout.
		// Once we've gone through the loop waiting for results
		// a couple of times, we'll close this to indicate that
		// a timeout occurred.
		if c.timeoutCh != nil {
			panic("time.After called for timeout multiple times")
		}
		c.timeoutCh = ch
	}
	return ch
}

func (s *ExecSuite) TestBlockAllMachines(c *gc.C) {
	mock := s.setupMockAPI()
	// Block operation
	mock.block = true
	_, err := cmdtesting.RunCommand(c, newTestExecCommand(&mockClock{}, model.IAAS), "--format=json", "--all", "hostname")
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *ExecSuite) TestSingleResponse(c *gc.C) {
	mock := s.setupMockAPI()
	mock.setMachinesAlive("0")
	mockResponse := mockResponse{
		stdout:     "stdout\n",
		stderr:     "stderr\n",
		code:       "42",
		machineTag: "machine-0",
	}
	mock.setResponse("0", mockResponse)

	machineResult := mock.execResponses["0"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["0"]: machineResult,
	}

	query := makeActionQuery(mock.receiverIdMap["0"], "MachineId", names.NewMachineTag("0"))
	unformatted := []interface{}{
		ConvertActionResults(machineResult, query, false),
	}

	jsonFormatted := &bytes.Buffer{}
	err := cmd.FormatJson(jsonFormatted, unformatted)
	c.Assert(err, jc.ErrorIsNil)

	yamlFormatted := &bytes.Buffer{}
	err = cmd.FormatYaml(yamlFormatted, unformatted)
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range []struct {
		message    string
		format     string
		stdout     string
		stderr     string
		errorMatch string
	}{{
		message:    "smart (default)",
		stdout:     "stdout\n",
		stderr:     "stderr\n",
		errorMatch: "subprocess encountered error code 42",
	}, {
		message: "yaml output",
		format:  "yaml",
		stdout:  yamlFormatted.String(),
	}, {
		message: "json output",
		format:  "json",
		stdout:  jsonFormatted.String(),
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		args := []string{}
		if test.format != "" {
			args = append(args, "--format", test.format)
		}
		args = append(args, "--all", "ignored")
		context, err := cmdtesting.RunCommand(c, newTestExecCommand(&mockClock{}, model.IAAS), args...)
		if test.errorMatch != "" {
			c.Check(err, gc.ErrorMatches, test.errorMatch)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(cmdtesting.Stdout(context), gc.Equals, test.stdout)
		c.Check(cmdtesting.Stderr(context), gc.Equals, test.stderr)
	}
}

func (s *ExecSuite) setupMockAPI() *mockExecAPI {
	mock := &mockExecAPI{
		bestAPIVersion: 4,
	}
	s.PatchValue(&getExecAPIClient, func(_ *execCommand) (ExecClient, error) {
		return mock, nil
	})
	return mock
}

type mockExecAPI struct {
	action.APIClient
	stdout string
	stderr string
	code   int
	// machines, applications, units
	machines        map[string]bool
	execResponses   map[string]params.ActionResult
	actionResponses map[string]params.ActionResult
	receiverIdMap   map[string]string
	block           bool
	//
	bestAPIVersion int
	// recevied values
	execParams *params.RunParams
}

type mockResponse struct {
	stdout     interface{}
	stderr     interface{}
	code       interface{}
	error      *params.Error
	message    string
	machineTag string
	unitTag    string
	status     string
}

var _ ExecClient = (*mockExecAPI)(nil)

func (m *mockExecAPI) setMachinesAlive(ids ...string) {
	if m.machines == nil {
		m.machines = make(map[string]bool)
	}
	for _, id := range ids {
		m.machines[id] = true
	}
}

func makeActionQuery(actionID string, receiverType string, receiverTag names.Tag) actionQuery {
	return actionQuery{
		actionTag: names.NewActionTag(actionID),
		receiver: actionReceiver{
			receiverType: receiverType,
			tag:          receiverTag,
		},
	}
}

func makeActionResult(mock mockResponse, actionTag string) params.ActionResult {
	var receiverTag string
	if mock.unitTag != "" {
		receiverTag = mock.unitTag
	} else {
		receiverTag = mock.machineTag
	}
	if actionTag == "" {
		actionTag = names.NewActionTag(utils.MustNewUUID().String()).String()
	}
	return params.ActionResult{
		Action: &params.Action{
			Tag:      actionTag,
			Receiver: receiverTag,
		},
		Message: mock.message,
		Status:  mock.status,
		Error:   mock.error,
		Output: map[string]interface{}{
			"stdout":      mock.stdout,
			"stderr":      mock.stderr,
			"return-code": mock.code,
		},
	}
}

func (m *mockExecAPI) setResponse(id string, mock mockResponse) {
	if m.execResponses == nil {
		m.execResponses = make(map[string]params.ActionResult)
	}
	if m.receiverIdMap == nil {
		m.receiverIdMap = make(map[string]string)
	}
	actionTag := names.NewActionTag(utils.MustNewUUID().String())
	m.receiverIdMap[id] = actionTag.Id()
	m.execResponses[id] = makeActionResult(mock, actionTag.String())
}

func (*mockExecAPI) Close() error {
	return nil
}

func (m *mockExecAPI) RunOnAllMachines(commands string, timeout time.Duration) ([]params.ActionResult, error) {
	var result []params.ActionResult

	if m.block {
		return result, common.OperationBlockedError("the operation has been blocked")
	}
	sortedMachineIds := make([]string, 0, len(m.machines))
	for machineId := range m.machines {
		sortedMachineIds = append(sortedMachineIds, machineId)
	}
	sort.Strings(sortedMachineIds)

	for _, machineId := range sortedMachineIds {
		response, found := m.execResponses[machineId]
		if !found {
			// Consider this a timeout
			response = params.ActionResult{
				Action: &params.Action{
					Receiver: names.NewMachineTag(machineId).String(),
				},
				Message: exec.ErrCancelled.Error(),
			}
		}
		result = append(result, response)
	}

	return result, nil
}

func (m *mockExecAPI) Run(runParams params.RunParams) ([]params.ActionResult, error) {
	var result []params.ActionResult

	m.execParams = &runParams

	if m.block {
		return result, common.OperationBlockedError("the operation has been blocked")
	}
	// Just add in ids that match in order.
	for _, id := range runParams.Machines {
		response, found := m.execResponses[id]
		if found {
			result = append(result, response)
		}
	}
	// mock ignores applications
	for _, id := range runParams.Units {
		response, found := m.execResponses[id]
		if found {
			result = append(result, response)
		}
	}

	return result, nil
}

func (m *mockExecAPI) Actions(actionTags params.Entities) (params.ActionResults, error) {
	results := params.ActionResults{Results: make([]params.ActionResult, len(actionTags.Entities))}

	for i, entity := range actionTags.Entities {
		response, found := m.actionResponses[entity.Tag[len("action-"):]]
		if !found {
			results.Results[i] = params.ActionResult{
				Error: &params.Error{
					Message: "action not found",
				},
			}
			continue
		}
		results.Results[i] = response
	}

	return results, nil
}

func (m *mockExecAPI) BestAPIVersion() int {
	return m.bestAPIVersion
}

// validUUID is a UUID used in tests
var validUUID = "01234567-89ab-cdef-0123-456789abcdef"
