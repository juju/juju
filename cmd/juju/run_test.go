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
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		runCmd := &RunCommand{}
		testing.TestInit(c, runCmd, test.args, test.errMatch)
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
