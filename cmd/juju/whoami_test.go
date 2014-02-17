// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	jujutesting "launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing"
)

type WhoamiSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&WhoamiSuite{})

func (s *WhoamiSuite) TestWhoami(c *gc.C) {
	environ, err := environs.PrepareFromName("dummyenv", s.ConfigStore)
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &AdduserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &LoginCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	//_, err = testing.RunCommand(c, &WhoamiCommand{}, []string{})
	ctx := coretesting.Context(c)
	returnCode := cmd.Main(&WhoamiCommand{}, ctx, []string{})
	c.Assert(returnCode, gc.Equals, 0)
	stdout := ctx.Stdout.(*bytes.Buffer).Bytes()
	c.Assert(string(stdout), gc.Equals, "foobar\n")

	err = environ.Destroy()
	c.Assert(err, gc.IsNil)
}
