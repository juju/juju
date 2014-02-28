// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type AddUserSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&AddUserSuite{})

func (s *AddUserSuite) Testadduser(c *gc.C) {

	_, err := testing.RunCommand(c, &AddUserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &AddUserCommand{}, []string{"foobar", "newpassword"})
	c.Assert(err, gc.ErrorMatches, "user already exists")
}
