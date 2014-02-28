// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	//"launchpad.net/juju-core/environs"
	jujutesting "launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"

//	"launchpad.net/juju-core/testing"
)

type WhoamiSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&WhoamiSuite{})

func (s *WhoamiSuite) TestWhoami(c *gc.C) {
	//environ, err := environs.PrepareFromName("dummyenv", nullContext(), s.ConfigStore)
	//c.Assert(err, gc.IsNil)

	_, err := coretesting.RunCommand(c, &AddUserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = coretesting.RunCommand(c, &LoginCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	//_, err = coretesting.RunCommand(c, &WhoamiCommand{}, []string{})
	ctx := coretesting.Context(c)
	returnCode := cmd.Main(&WhoamiCommand{}, ctx, []string{})
	c.Assert(returnCode, gc.Equals, 0)
	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, gc.Equals, "foobar\n")

	//err = environ.Destroy()
	//c.Assert(err, gc.IsNil)
}
