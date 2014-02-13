// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type WhoamiSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&WhoamiSuite{})

func (s *WhoamiSuite) Testadduser(c *gc.C) {

	_, err := testing.RunCommand(c, &AdduserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &LoginCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &WhoamiCommand{}, []string{"foobar"})
	c.Assert(err, gc.IsNil)
}
