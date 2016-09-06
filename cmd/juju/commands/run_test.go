// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/testing"
)

type RunSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&RunSuite{})

func (*RunSuite) TestTargetArgParsing(c *gc.C) {
	for i, test := range []struct {
		message  string
		args     []string
		all      bool
		machines []string
		units    []string
		services []string
		commands string
		errMatch string
	}{{
		message:  "no args",
		errMatch: "no commands specified",
	}, {
		message:  "no target",
		args:     []string{"sudo reboot"},
		errMatch: "You must specify a target, either through --all, --machine, --application or --unit",
	}, {
		message:  "too many args",
		args:     []string{"--all", "sudo reboot", "oops"},
		errMatch: `unrecognized args: \["oops"\]`,
	}, {
		message:  "command to all machines",
		args:     []string{"--all", "sudo reboot"},
		all:      true,
		commands: "sudo reboot",
	}, {
		message:  "all and defined machines",
		args:     []string{"--all", "--machine=1,2", "sudo reboot"},
		errMatch: `You cannot specify --all and individual machines`,
	}, {
		message:  "command to machines 1, 2, and 1/kvm/0",
		args:     []string{"--machine=1,2,1/kvm/0", "sudo reboot"},
		commands: "sudo reboot",
		machines: []string{"1", "2", "1/kvm/0"},
	}, {
		message: "bad machine names",
		args:    []string{"--machine=foo,machine-2", "sudo reboot"},
		errMatch: "" +
			"The following run targets are not valid:\n" +
			"  \"foo\" is not a valid machine id\n" +
			"  \"machine-2\" is not a valid machine id",
	}, {
		message:  "all and defined applications",
		args:     []string{"--all", "--application=wordpress,mysql", "sudo reboot"},
		errMatch: `You cannot specify --all and individual applications`,
	}, {
		message:  "command to applications wordpress and mysql",
		args:     []string{"--application=wordpress,mysql", "sudo reboot"},
		commands: "sudo reboot",
		services: []string{"wordpress", "mysql"},
	}, {
		message: "bad application names",
		args:    []string{"--application", "foo,2,foo/0", "sudo reboot"},
		errMatch: "" +
			"The following run targets are not valid:\n" +
			"  \"2\" is not a valid application name\n" +
			"  \"foo/0\" is not a valid application name",
	}, {
		message:  "all and defined units",
		args:     []string{"--all", "--unit=wordpress/0,mysql/1", "sudo reboot"},
		errMatch: `You cannot specify --all and individual units`,
	}, {
		message:  "command to valid units",
		args:     []string{"--unit=wordpress/0,wordpress/1,mysql/0", "sudo reboot"},
		commands: "sudo reboot",
		units:    []string{"wordpress/0", "wordpress/1", "mysql/0"},
	}, {
		message: "bad unit names",
		args:    []string{"--unit", "foo,2,foo/0", "sudo reboot"},
		errMatch: "" +
			"The following run targets are not valid:\n" +
			"  \"foo\" is not a valid unit name\n" +
			"  \"2\" is not a valid unit name",
	}, {
		message:  "command to mixed valid targets",
		args:     []string{"--machine=0", "--unit=wordpress/0,wordpress/1", "--application=mysql", "sudo reboot"},
		commands: "sudo reboot",
		machines: []string{"0"},
		services: []string{"mysql"},
		units:    []string{"wordpress/0", "wordpress/1"},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		cmd := &runCommand{}
		runCmd := modelcmd.Wrap(cmd)
		testing.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(cmd.all, gc.Equals, test.all)
			c.Check(cmd.machines, gc.DeepEquals, test.machines)
			c.Check(cmd.services, gc.DeepEquals, test.services)
			c.Check(cmd.units, gc.DeepEquals, test.units)
			c.Check(cmd.commands, gc.Equals, test.commands)
		}
	}
}

func (*RunSuite) TestTimeoutArgParsing(c *gc.C) {
	for i, test := range []struct {
		message  string
		args     []string
		errMatch string
		timeout  time.Duration
	}{{
		message: "default time",
		args:    []string{"--all", "sudo reboot"},
		timeout: 5 * time.Minute,
	}, {
		message:  "invalid time",
		args:     []string{"--timeout=foo", "--all", "sudo reboot"},
		errMatch: `invalid value "foo" for flag --timeout: time: invalid duration foo`,
	}, {
		message: "two hours",
		args:    []string{"--timeout=2h", "--all", "sudo reboot"},
		timeout: 2 * time.Hour,
	}, {
		message: "3 minutes 30 seconds",
		args:    []string{"--timeout=3m30s", "--all", "sudo reboot"},
		timeout: (3 * time.Minute) + (30 * time.Second),
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		cmd := &runCommand{}
		runCmd := modelcmd.Wrap(cmd)
		testing.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(cmd.timeout, gc.Equals, test.timeout)
		}
	}
}

func (s *RunSuite) TestConvertRunResults(c *gc.C) {
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
			"Stdout":    "",
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
			"UnitId":     "unit/0",
			"Stdout":     "stdout",
			"Stderr":     "stderr",
			"Message":    "msg",
			"ReturnCode": 42,
		},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		result := ConvertActionResults(test.results, test.query)
		c.Check(result, jc.DeepEquals, test.expected)
	}
}

func (s *RunSuite) TestRunForMachineAndUnit(c *gc.C) {
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

	machineResult := mock.runResponses["0"]
	unitResult := mock.runResponses["unit/0"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["0"]:      machineResult,
		mock.receiverIdMap["unit/0"]: unitResult,
	}

	machineQuery := makeActionQuery(mock.receiverIdMap["0"], "MachineId", names.NewMachineTag("0"))
	unitQuery := makeActionQuery(mock.receiverIdMap["unit/0"], "UnitId", names.NewUnitTag("unit/0"))
	unformatted := []interface{}{
		ConvertActionResults(machineResult, machineQuery),
		ConvertActionResults(unitResult, unitQuery),
	}

	buff := &bytes.Buffer{}
	err := cmd.FormatJson(buff, unformatted)
	c.Assert(err, jc.ErrorIsNil)

	context, err := testing.RunCommand(c, newRunCommand(),
		"--format=json", "--machine=0", "--unit=unit/0", "hostname",
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(testing.Stdout(context), gc.Equals, buff.String())
}

func (s *RunSuite) TestBlockRunForMachineAndUnit(c *gc.C) {
	mock := s.setupMockAPI()
	// Block operation
	mock.block = true
	_, err := testing.RunCommand(c, newRunCommand(),
		"--format=json", "--machine=0", "--unit=unit/0", "hostname",
	)
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To enable changes.*")
}

func (s *RunSuite) TestAllMachines(c *gc.C) {
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

	machine0Result := mock.runResponses["0"]
	machine1Result := mock.runResponses["1"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["0"]: machine0Result,
		mock.receiverIdMap["1"]: machine1Result,
	}

	machine0Query := makeActionQuery(mock.receiverIdMap["0"], "MachineId", names.NewMachineTag("0"))
	machine1Query := makeActionQuery(mock.receiverIdMap["1"], "MachineId", names.NewMachineTag("1"))
	unformatted := []interface{}{
		ConvertActionResults(machine0Result, machine0Query),
		ConvertActionResults(machine1Result, machine1Query),
		map[string]interface{}{
			"Action":    mock.receiverIdMap["2"],
			"MachineId": "2",
			"Error":     "action not found",
		},
	}

	buff := &bytes.Buffer{}
	err := cmd.FormatJson(buff, unformatted)
	c.Assert(err, jc.ErrorIsNil)

	context, err := testing.RunCommand(c, newRunCommand(), "--format=json", "--all", "hostname")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(testing.Stdout(context), gc.Equals, buff.String())
	c.Check(testing.Stderr(context), gc.Equals, "")
}

func (s *RunSuite) TestBlockAllMachines(c *gc.C) {
	mock := s.setupMockAPI()
	// Block operation
	mock.block = true
	_, err := testing.RunCommand(c, newRunCommand(), "--format=json", "--all", "hostname")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To enable changes.*")
}

func (s *RunSuite) TestSingleResponse(c *gc.C) {
	mock := s.setupMockAPI()
	mock.setMachinesAlive("0")
	mockResponse := mockResponse{
		stdout:     "stdout\n",
		stderr:     "stderr\n",
		code:       "42",
		machineTag: "machine-0",
	}
	mock.setResponse("0", mockResponse)

	machineResult := mock.runResponses["0"]
	mock.actionResponses = map[string]params.ActionResult{
		mock.receiverIdMap["0"]: machineResult,
	}

	query := makeActionQuery(mock.receiverIdMap["0"], "MachineId", names.NewMachineTag("0"))
	unformatted := []interface{}{
		ConvertActionResults(machineResult, query),
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
		context, err := testing.RunCommand(c, newRunCommand(), args...)
		if test.errorMatch != "" {
			c.Check(err, gc.ErrorMatches, test.errorMatch)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(testing.Stdout(context), gc.Equals, test.stdout)
		c.Check(testing.Stderr(context), gc.Equals, test.stderr)
	}
}

func (s *RunSuite) setupMockAPI() *mockRunAPI {
	mock := &mockRunAPI{}
	s.PatchValue(&getRunAPIClient, func(_ *runCommand) (RunClient, error) {
		return mock, nil
	})
	return mock
}

type mockRunAPI struct {
	action.APIClient
	stdout string
	stderr string
	code   int
	// machines, services, units
	machines        map[string]bool
	runResponses    map[string]params.ActionResult
	actionResponses map[string]params.ActionResult
	receiverIdMap   map[string]string
	block           bool
}

type mockResponse struct {
	stdout     interface{}
	stderr     interface{}
	code       interface{}
	error      *params.Error
	message    string
	machineTag string
	unitTag    string
}

var _ RunClient = (*mockRunAPI)(nil)

func (m *mockRunAPI) setMachinesAlive(ids ...string) {
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
		Error:   mock.error,
		Output: map[string]interface{}{
			"Stdout": mock.stdout,
			"Stderr": mock.stderr,
			"Code":   mock.code,
		},
	}
}

func (m *mockRunAPI) setResponse(id string, mock mockResponse) {
	if m.runResponses == nil {
		m.runResponses = make(map[string]params.ActionResult)
	}
	if m.receiverIdMap == nil {
		m.receiverIdMap = make(map[string]string)
	}
	actionTag := names.NewActionTag(utils.MustNewUUID().String())
	m.receiverIdMap[id] = actionTag.Id()
	m.runResponses[id] = makeActionResult(mock, actionTag.String())
}

func (*mockRunAPI) Close() error {
	return nil
}

func (m *mockRunAPI) RunOnAllMachines(commands string, timeout time.Duration) ([]params.ActionResult, error) {
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
		response, found := m.runResponses[machineId]
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

func (m *mockRunAPI) Run(runParams params.RunParams) ([]params.ActionResult, error) {
	var result []params.ActionResult

	if m.block {
		return result, common.OperationBlockedError("the operation has been blocked")
	}
	// Just add in ids that match in order.
	for _, id := range runParams.Machines {
		response, found := m.runResponses[id]
		if found {
			result = append(result, response)
		}
	}
	// mock ignores services
	for _, id := range runParams.Units {
		response, found := m.runResponses[id]
		if found {
			result = append(result, response)
		}
	}

	return result, nil
}

func (m *mockRunAPI) Actions(actionTags params.Entities) (params.ActionResults, error) {
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

// validUUID is a UUID used in tests
var validUUID = "01234567-89ab-cdef-0123-456789abcdef"
