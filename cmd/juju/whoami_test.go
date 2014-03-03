// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	jujutesting "launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type WhoamiSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&WhoamiSuite{})

func (s *WhoamiSuite) TestWhoami(c *gc.C) {
	_, err := coretesting.RunCommand(c, &AddUserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = coretesting.RunCommand(c, &LoginCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	ctx := coretesting.Context(c)
	returnCode := cmd.Main(&WhoamiCommand{}, ctx, []string{})
	c.Assert(returnCode, gc.Equals, 0)
	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, gc.Equals, "foobar\n")
}
