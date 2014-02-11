// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type AdduserSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&AdduserSuite{})

func runAdduser(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, &AdduserCommand{}, args)
	return err
}

func (s *AdduserSuite) Testadduser(c *gc.C) {

	err := runAdduser(c, "foobar", "password")
	c.Assert(err, gc.IsNil)

	err = runAdduser(c, "foobar", "password")
	c.Assert(err, gc.ErrorMatches, "user already exists")
}
