// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type RunTestSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&RunTestSuite{})

//func (s *RunTestSuite) SetUpTest(c *gc.C) {
//}

func (*RunTestSuite) TestWrongArgs(c *gc.C) {
	for i, test := range []struct {
		title    string
		args     []string
		errMatch string
		unit     string
		commands string
	}{{
		title:    "no args",
		errMatch: "missing unit-name",
	},
	} {
		c.Log(fmt.Sprintf("\n%d: %s", i, test.title))
		runCommand := &RunCommand{}
		err := runCommand.Init(test.args)
		if test.errMatch == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(runCommand.unit, gc.Equals, test.unit)
			c.Assert(runCommand.commands, gc.Equals, test.commands)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}
