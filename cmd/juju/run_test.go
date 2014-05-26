// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/exec"
)

type RunSuite struct {
	testing.FakeJujuHomeSuite
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
		errMatch: "You must specify a target, either through --all, --machine, --service or --unit",
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
		message:  "all and defined services",
		args:     []string{"--all", "--service=wordpress,mysql", "sudo reboot"},
		errMatch: `You cannot specify --all and individual services`,
	}, {
		message:  "command to services wordpress and mysql",
		args:     []string{"--service=wordpress,mysql", "sudo reboot"},
		commands: "sudo reboot",
		services: []string{"wordpress", "mysql"},
	}, {
		message: "bad service names",
		args:    []string{"--service", "foo,2,foo/0", "sudo reboot"},
		errMatch: "" +
			"The following run targets are not valid:\n" +
			"  \"2\" is not a valid service name\n" +
			"  \"foo/0\" is not a valid service name",
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
		args:     []string{"--machine=0", "--unit=wordpress/0,wordpress/1", "--service=mysql", "sudo reboot"},
		commands: "sudo reboot",
		machines: []string{"0"},
		services: []string{"mysql"},
		units:    []string{"wordpress/0", "wordpress/1"},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		runCmd := &RunCommand{}
		testing.TestInit(c, envcmd.Wrap(runCmd), test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(runCmd.all, gc.Equals, test.all)
			c.Check(runCmd.machines, gc.DeepEquals, test.machines)
			c.Check(runCmd.services, gc.DeepEquals, test.services)
			c.Check(runCmd.units, gc.DeepEquals, test.units)
			c.Check(runCmd.commands, gc.Equals, test.commands)
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
		runCmd := &RunCommand{}
		testing.TestInit(c, envcmd.Wrap(runCmd), test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(runCmd.timeout, gc.Equals, test.timeout)
		}
	}
}

func (s *RunSuite) TestConvertRunResults(c *gc.C) {
	for i, test := range []struct {
		message  string
		results  []params.RunResult
		expected interface{}
	}{{
		message:  "empty",
		expected: []interface{}{},
	}, {
		message: "minimum is machine id and stdout",
		results: []params.RunResult{
			makeRunResult(mockResponse{machineId: "1"}),
		},
		expected: []interface{}{
			map[string]interface{}{
				"MachineId": "1",
				"Stdout":    "",
			}},
	}, {
		message: "other fields are copied if there",
		results: []params.RunResult{
			makeRunResult(mockResponse{
				machineId: "1",
				stdout:    "stdout",
				stderr:    "stderr",
				code:      42,
				unitId:    "unit/0",
				error:     "error",
			}),
		},
		expected: []interface{}{
			map[string]interface{}{
				"MachineId":  "1",
				"Stdout":     "stdout",
				"Stderr":     "stderr",
				"ReturnCode": 42,
				"UnitId":     "unit/0",
				"Error":      "error",
			}},
	}, {
		message: "stdout and stderr are base64 encoded if not valid utf8",
		results: []params.RunResult{
			params.RunResult{
				ExecResponse: exec.ExecResponse{
					Stdout: []byte{0xff},
					Stderr: []byte{0xfe},
				},
				MachineId: "jake",
			},
		},
		expected: []interface{}{
			map[string]interface{}{
				"MachineId":       "jake",
				"Stdout":          "/w==",
				"Stdout.encoding": "base64",
				"Stderr":          "/g==",
				"Stderr.encoding": "base64",
			}},
	}, {
		message: "more than one",
		results: []params.RunResult{
			makeRunResult(mockResponse{machineId: "1"}),
			makeRunResult(mockResponse{machineId: "2"}),
			makeRunResult(mockResponse{machineId: "3"}),
		},
		expected: []interface{}{
			map[string]interface{}{
				"MachineId": "1",
				"Stdout":    "",
			},
			map[string]interface{}{
				"MachineId": "2",
				"Stdout":    "",
			},
			map[string]interface{}{
				"MachineId": "3",
				"Stdout":    "",
			},
		},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		result := ConvertRunResults(test.results)
		c.Check(result, jc.DeepEquals, test.expected)
	}
}

func (s *RunSuite) TestRunForMachineAndUnit(c *gc.C) {
	mock := s.setupMockAPI()
	machineResponse := mockResponse{
		stdout:    "megatron\n",
		machineId: "0",
	}
	unitResponse := mockResponse{
		stdout:    "bumblebee",
		machineId: "1",
		unitId:    "unit/0",
	}
	mock.setResponse("0", machineResponse)
	mock.setResponse("unit/0", unitResponse)

	unformatted := ConvertRunResults([]params.RunResult{
		makeRunResult(machineResponse),
		makeRunResult(unitResponse),
	})

	jsonFormatted, err := cmd.FormatJson(unformatted)
	c.Assert(err, gc.IsNil)

	context, err := testing.RunCommand(c, envcmd.Wrap(&RunCommand{}),
		"--format=json", "--machine=0", "--unit=unit/0", "hostname",
	)
	c.Assert(err, gc.IsNil)

	c.Check(testing.Stdout(context), gc.Equals, string(jsonFormatted)+"\n")
}

func (s *RunSuite) TestAllMachines(c *gc.C) {
	mock := s.setupMockAPI()
	mock.setMachinesAlive("0", "1")
	response0 := mockResponse{
		stdout:    "megatron\n",
		machineId: "0",
	}
	response1 := mockResponse{
		error:     "command timed out",
		machineId: "1",
	}
	mock.setResponse("0", response0)

	unformatted := ConvertRunResults([]params.RunResult{
		makeRunResult(response0),
		makeRunResult(response1),
	})

	jsonFormatted, err := cmd.FormatJson(unformatted)
	c.Assert(err, gc.IsNil)

	context, err := testing.RunCommand(c, &RunCommand{}, "--format=json", "--all", "hostname")
	c.Assert(err, gc.IsNil)

	c.Check(testing.Stdout(context), gc.Equals, string(jsonFormatted)+"\n")
}

func (s *RunSuite) TestSingleResponse(c *gc.C) {
	mock := s.setupMockAPI()
	mock.setMachinesAlive("0")
	mockResponse := mockResponse{
		stdout:    "stdout\n",
		stderr:    "stderr\n",
		code:      42,
		machineId: "0",
	}
	mock.setResponse("0", mockResponse)
	unformatted := ConvertRunResults([]params.RunResult{
		makeRunResult(mockResponse)})
	yamlFormatted, err := cmd.FormatYaml(unformatted)
	c.Assert(err, gc.IsNil)
	jsonFormatted, err := cmd.FormatJson(unformatted)
	c.Assert(err, gc.IsNil)

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
		stdout:  string(yamlFormatted) + "\n",
	}, {
		message: "json output",
		format:  "json",
		stdout:  string(jsonFormatted) + "\n",
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		args := []string{}
		if test.format != "" {
			args = append(args, "--format", test.format)
		}
		args = append(args, "--all", "ignored")
		context, err := testing.RunCommand(c, envcmd.Wrap(&RunCommand{}), args...)
		if test.errorMatch != "" {
			c.Check(err, gc.ErrorMatches, test.errorMatch)
		} else {
			c.Check(err, gc.IsNil)
		}
		c.Check(testing.Stdout(context), gc.Equals, test.stdout)
		c.Check(testing.Stderr(context), gc.Equals, test.stderr)
	}
}

func (s *RunSuite) setupMockAPI() *mockRunAPI {
	mock := &mockRunAPI{}
	s.PatchValue(&getAPIClient, func(name string) (RunClient, error) {
		return mock, nil
	})
	return mock
}

type mockRunAPI struct {
	stdout string
	stderr string
	code   int
	// machines, services, units
	machines  map[string]bool
	responses map[string]params.RunResult
}

type mockResponse struct {
	stdout    string
	stderr    string
	code      int
	error     string
	machineId string
	unitId    string
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

func makeRunResult(mock mockResponse) params.RunResult {
	return params.RunResult{
		ExecResponse: exec.ExecResponse{
			Stdout: []byte(mock.stdout),
			Stderr: []byte(mock.stderr),
			Code:   mock.code,
		},
		MachineId: mock.machineId,
		UnitId:    mock.unitId,
		Error:     mock.error,
	}
}

func (m *mockRunAPI) setResponse(id string, mock mockResponse) {
	if m.responses == nil {
		m.responses = make(map[string]params.RunResult)
	}
	m.responses[id] = makeRunResult(mock)
}

func (*mockRunAPI) Close() error {
	return nil
}

func (m *mockRunAPI) RunOnAllMachines(commands string, timeout time.Duration) ([]params.RunResult, error) {
	var result []params.RunResult
	for machine := range m.machines {
		response, found := m.responses[machine]
		if !found {
			// Consider this a timeout
			response = params.RunResult{MachineId: machine, Error: "command timed out"}
		}
		result = append(result, response)
	}

	return result, nil
}

func (m *mockRunAPI) Run(runParams params.RunParams) ([]params.RunResult, error) {
	var result []params.RunResult
	// Just add in ids that match in order.
	for _, id := range runParams.Machines {
		response, found := m.responses[id]
		if found {
			result = append(result, response)
		}
	}
	// mock ignores services
	for _, id := range runParams.Units {
		response, found := m.responses[id]
		if found {
			result = append(result, response)
		}
	}

	return result, nil
}
