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
	}, {
		title:    "one arg",
		args:     []string{"foo"},
		errMatch: "missing commands",
	}, {
		title:    "more than two arg",
		args:     []string{"foo", "bar", "baz"},
		errMatch: `unrecognized args: \["baz"\]`,
	}, {
		title:    "unit and command assignment",
		args:     []string{"unit-name", "command"},
		unit:     "unit-name",
		commands: "command",
	}, {
		title:    "unit id converted to tag",
		args:     []string{"foo/1", "command"},
		unit:     "unit-foo-1",
		commands: "command",
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

func (s *RunTestSuite) TestInsideContext(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTEXT_ID", "fake-id")
	runCommand := &RunCommand{}
	err := runCommand.Init([]string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, "juju-run cannot be called from within a hook.*")
}
