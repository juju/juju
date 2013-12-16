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

type mockRunAPI struct {
	stdout string
	stderr string
	code   int
	// machines, services, units
}

var _ RunClient = (*mockRunAPI)(nil)

func (*mockRunAPI) Close() error {
	return nil
}

func (*mockRunAPI) RunOnAllMachines(commands string, timeout time.Duration) ([]api.RunResult, error) {
	return nil, fmt.Errorf("todo")
}
func (*mockRunAPI) Run(params api.RunParams) ([]api.RunResult, error) {
	return nil, fmt.Errorf("todo")
}
