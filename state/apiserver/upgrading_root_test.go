// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/testing"
)

type upgradingRootSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&upgradingRootSuite{})

func (r *upgradingRootSuite) TestFindAllowedMethod(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)

	caller, err := root.FindMethod("Client", 0, "FullStatus")

	c.Assert(err, gc.IsNil)
	c.Assert(caller, gc.NotNil)
}

func (r *upgradingRootSuite) TestFindDisallowedMethod(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)

	caller, err := root.FindMethod("Client", 0, "ServiceDeploy")

	c.Assert(err, gc.ErrorMatches, "upgrade in progress - Juju functionality is limited")
	c.Assert(caller, gc.IsNil)
}

func (r *upgradingRootSuite) TestFindNonExistentMethod(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)

	caller, err := root.FindMethod("Foo", 0, "Bar")

	c.Assert(err, gc.ErrorMatches, "unknown object type \"Foo\"")
	c.Assert(caller, gc.IsNil)
}

func (r *upgradingRootSuite) TestFindMethodNonExistentVersion(c *gc.C) {
	root := apiserver.TestingUpgradingRoot(nil)

	caller, err := root.FindMethod("Client", 99999999, "Status")

	c.Assert(err, gc.ErrorMatches, "unknown version \\(99999999\\) of interface \"Client\"")
	c.Assert(caller, gc.IsNil)
}
