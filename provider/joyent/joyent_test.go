// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/provider/joyent"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type JoyentSuite struct{}

var _ = gc.Suite(&JoyentSuite{})

func (*JoyentSuite) TestRegistered(c *gc.C) {
	provider, err := environs.Provider("joyent")
	c.Assert(err, gc.IsNil)
	c.Assert(provider, gc.Equals, joyent.Provider)
}
