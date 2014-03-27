// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type RemoveUserSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&RemoveUserSuite{})

func (s *RemoveUserSuite) TestRemoveUser(c *gc.C) {
	_, err := testing.RunCommand(c, &AddUserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &RemoveUserCommand{}, []string{"foobar"})
	c.Assert(err, gc.IsNil)
}

func (s *RemoveUserSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, &RemoveUserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["password"\]`)
}

func (s *RemoveUserSuite) TestNotEnoughArgs(c *gc.C) {
	_, err := testing.RunCommand(c, &RemoveUserCommand{}, []string{})
	c.Assert(err, gc.ErrorMatches, `no username supplied`)
}
