// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing/testbase"
)

type RunSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&RunSuite{})

func (*RunSuite) TestArgParsing(c *gc.C) {

	c.Fail()
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
