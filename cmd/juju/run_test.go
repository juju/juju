// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
)

type RunSuite struct {
	testing.FakeHomeSuite
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
		args:     []string{"--all", "--service=wordpress, mysql", "sudo reboot"},
		errMatch: `You cannot specify --all and individual services`,
	}, {
		message:  "command to services wordpress and mysql",
		args:     []string{"--service=wordpress, mysql", "sudo reboot"},
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
		args:     []string{"--all", "--unit=wordpress/0, mysql/1", "sudo reboot"},
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
		testing.TestInit(c, runCmd, test.args, test.errMatch)
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
		testing.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(runCmd.timeout, gc.Equals, test.timeout)
		}
	}
}

func (s *RunSuite) TestAllMachines(c *gc.C) {
	mock = &mockRunAPI{}
	s.PatchValue(&getAPIClient, func(name string) (RunClient, error) {
		return mock, nil
	})

}

type mockRunAPI struct {
	stdout string
	stderr string
	code   int
	// machines, services, units
	machines map[string]bool
	responses map[string]api.RunResult
}

var _ RunClient = (*mockRunAPI)(nil)

func (m *mockRunAPI) setMachinesAlive(ids ...string)  {
	if m.machines == nil {
		m.machines = make(map[string]bool)
	}
	for _, id := range ids {
		m.machines[id] = true
	}
}

func (m *mockRunAPI) setResponse(id string, result api.RunResult) {
	if m.responses == nil {
		m.responses = make(map[string]api.RunResult)
	}
	m.responses[id] = result
}

func (*mockRunAPI) Close() error {
	return nil
}

func (m *mockRunAPI) RunOnAllMachines(commands string, timeout time.Duration) ([]api.RunResult, error) {
	var result []api.RunResult
	for machine := range m.machines {
		response, found := r.responses[machine]
		if !found {
			// Consider this a timeout
			response = api.RunResult{MachineId: machine, Error: fmt.Errorf("command timed out")}
		}
		result = append(result, respose)
	}

	return result, nil
}
func (*mockRunAPI) Run(params api.RunParams) ([]api.RunResult, error) {
	return nil, fmt.Errorf("todo")
}
