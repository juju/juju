// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type LoginSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&LoginSuite{})

func (s *LoginSuite) Testadduser(c *gc.C) {
	environ, err := environs.PrepareFromName("dummyenv", s.ConfigStore)
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &AdduserCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	_, err = testing.RunCommand(c, &LoginCommand{}, []string{"foobar", "password"})
	c.Assert(err, gc.IsNil)

	info, err := s.ConfigStore.ReadInfo("dummyenv")
	c.Assert(info.APICredentials().User, gc.Equals, "foobar")

	err = environ.Destroy()
	c.Assert(err, gc.IsNil)
}
